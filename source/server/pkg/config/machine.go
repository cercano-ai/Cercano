package config

import (
	"context"
	"reflect"
	"sync"
	"time"

	"cercano/source/server/internal/filelock"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"cercano/source/server/pkg/paths"

	"gopkg.in/yaml.v3"
)

// MachineConfig holds the settings that describe this machine's local model
// runtimes. They are shared by every profile and live in machine.yaml, next to
// the profile directories, because one model host serves all profiles.
type MachineConfig struct {
	LlamaServer LlamaServerConfig `yaml:"llama_server"`
	MistralRS   MistralRSConfig   `yaml:"mistralrs"`
}

// machineDefaults returns the machine settings of Defaults().
func machineDefaults() MachineConfig {
	d := Defaults()
	return MachineConfig{LlamaServer: d.LlamaServer, MistralRS: d.MistralRS}
}

// LoadMachine reads machine.yaml. A missing file yields the defaults; fields
// absent from the file keep their default values.
func LoadMachine() (MachineConfig, error) {
	mc := machineDefaults()
	path := paths.MachineConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mc, nil
		}
		return mc, fmt.Errorf("failed to read machine config %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &mc); err != nil {
		return mc, fmt.Errorf("failed to parse machine config %q: %w", path, err)
	}
	d := machineDefaults()
	applyLlamaServerDefaults(&mc.LlamaServer, d.LlamaServer)
	applyMistralRSDefaults(&mc.MistralRS, d.MistralRS)
	return mc, nil
}

// SaveMachine writes machine.yaml atomically (temp file and rename) with mode
// 0600, creating the parent directory with mode 0700.
func SaveMachine(mc MachineConfig) error {
	if err := mc.LlamaServer.Validate(); err != nil {
		return err
	}
	path := paths.MachineConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory %q: %w", dir, err)
	}
	data, err := yaml.Marshal(mc)
	if err != nil {
		return fmt.Errorf("failed to marshal machine config: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".machine-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp machine config: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("failed to write machine config: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		tmp.Close()
		cleanup()
		return fmt.Errorf("failed to set machine config mode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("failed to close machine config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("failed to replace machine config %q: %w", path, err)
	}
	return nil
}

// machineFileExists reports whether machine.yaml is present.
func machineFileExists() bool {
	_, err := os.Stat(paths.MachineConfigPath())
	return err == nil
}

// machineBaseline records the machine sections a config last read from, or
// wrote to, machine.yaml.
type machineBaseline struct {
	mu sync.Mutex
	mc MachineConfig
}

func cloneMachine(mc MachineConfig) MachineConfig {
	c := Config{LlamaServer: mc.LlamaServer, MistralRS: mc.MistralRS}.Clone()
	return MachineConfig{LlamaServer: c.LlamaServer, MistralRS: c.MistralRS}
}

// overlayMachine replaces the runtime sections of cfg with machine.yaml's and
// records them as the baseline. When machine.yaml does not exist, cfg keeps
// whatever the profile file held (an older version wrote these sections
// there) so nothing is lost before migration; the baseline is then the
// defaults, so the first Save writes those legacy values to machine.yaml.
func overlayMachine(cfg *Config) error {
	if !machineFileExists() {
		cfg.machineBase = &machineBaseline{mc: machineDefaults()}
		return nil
	}
	mc, err := LoadMachine()
	if err != nil {
		return err
	}
	cfg.LlamaServer = mc.LlamaServer
	cfg.MistralRS = mc.MistralRS
	cfg.machineBase = &machineBaseline{mc: cloneMachine(mc)}
	return nil
}

// SetMachineSections replaces the runtime sections with mc (typically freshly
// read from machine.yaml) and makes them the baseline, so the next Save does
// not treat them as this config's own edits.
func (c *Config) SetMachineSections(mc MachineConfig) {
	mc = cloneMachine(mc)
	c.LlamaServer = mc.LlamaServer
	c.MistralRS = mc.MistralRS
	if c.machineBase == nil {
		c.machineBase = &machineBaseline{}
	}
	c.machineBase.mu.Lock()
	c.machineBase.mc = cloneMachine(mc)
	c.machineBase.mu.Unlock()
}

// MachineSectionsEqual reports whether the runtime sections equal mc.
func (c Config) MachineSectionsEqual(mc MachineConfig) bool {
	return reflect.DeepEqual(MachineConfig{LlamaServer: c.LlamaServer, MistralRS: c.MistralRS}, mc)
}

// mergeChangedFields copies into dst every field of cur that differs from
// base. All three are pointers to the same struct type. Fields compare as
// whole values, so slices and nested structs are replaced, not merged.
func mergeChangedFields(dst, cur, base any) {
	d := reflect.ValueOf(dst).Elem()
	c := reflect.ValueOf(cur).Elem()
	b := reflect.ValueOf(base).Elem()
	for i := 0; i < d.NumField(); i++ {
		if !d.Field(i).CanSet() {
			continue
		}
		if !reflect.DeepEqual(c.Field(i).Interface(), b.Field(i).Interface()) {
			d.Field(i).Set(c.Field(i))
		}
	}
}

// saveMachineChanges writes to machine.yaml only the fields this config
// changed relative to its baseline, merged onto the file's current content
// under a cross-process lock, so another profile's concurrent edits to other
// fields survive.
func (c *Config) saveMachineChanges() error {
	cur := MachineConfig{LlamaServer: c.LlamaServer, MistralRS: c.MistralRS}
	base := c.machineBase
	if base == nil {
		// Built without Load: compare against the defaults and leave no
		// baseline behind.
		base = &machineBaseline{mc: machineDefaults()}
	}
	base.mu.Lock()
	defer base.mu.Unlock()
	if reflect.DeepEqual(cur, base.mc) {
		return nil
	}
	lockPath := filepath.Join(filepath.Dir(paths.MachineConfigPath()), "machine.lock")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	release, err := filelock.Lock(ctx, lockPath)
	if err != nil {
		return fmt.Errorf("failed to lock machine config: %w", err)
	}
	defer release()
	merged, err := LoadMachine()
	if err != nil {
		return err
	}
	mergeChangedFields(&merged.LlamaServer, &cur.LlamaServer, &base.mc.LlamaServer)
	mergeChangedFields(&merged.MistralRS, &cur.MistralRS, &base.mc.MistralRS)
	if err := SaveMachine(merged); err != nil {
		return err
	}
	base.mc = cloneMachine(merged)
	return nil
}

// stripMachineKeys removes the machine-wide sections from marshaled profile
// YAML.
func stripMachineKeys(data []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return data, nil
	}
	m := doc.Content[0]
	kept := m.Content[:0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		switch m.Content[i].Value {
		case "llama_server", "mistralrs":
			continue
		}
		kept = append(kept, m.Content[i], m.Content[i+1])
	}
	m.Content = kept
	return yaml.Marshal(&doc)
}
