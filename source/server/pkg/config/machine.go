package config

import (
	"context"
	"reflect"
	"time"

	"cercano/source/server/internal/filelock"
	"errors"
	"fmt"
	"log"
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

// machineBaseline is the machine.yaml content a config copy last saw. It is a
// value carried by each Config copy; nothing is shared, so every snapshot
// compares only against what it itself saw. An unset baseline means the
// defaults.
type machineBaseline struct {
	set bool
	mc  MachineConfig
}

func newBaseline(mc MachineConfig) machineBaseline {
	return machineBaseline{set: true, mc: cloneMachine(mc)}
}

func (b machineBaseline) get() MachineConfig {
	if !b.set {
		return machineDefaults()
	}
	return b.mc
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
//
// An unreadable or malformed machine.yaml is logged, not returned: callers
// fall back to Defaults() on a Load error, and their next save would replace
// the profile's settings. cfg keeps the profile's values, and they become
// the baseline, so a save without runtime edits leaves machine.yaml alone;
// a runtime edit re-reads machine.yaml under the lock and fails until the
// file is fixed.
func overlayMachine(cfg *Config) {
	if !machineFileExists() {
		cfg.machineBase = newBaseline(machineDefaults())
		return
	}
	mc, err := LoadMachine()
	if err != nil {
		log.Printf("[config] ignoring the machine config: %v; using this profile's local runtime settings until it is fixed", err)
		cfg.machineBase = newBaseline(cfg.MachineSections())
		return
	}
	cfg.LlamaServer = mc.LlamaServer
	cfg.MistralRS = mc.MistralRS
	cfg.machineBase = newBaseline(mc)
}

// SetMachineSections replaces the runtime sections with mc (typically freshly
// read from machine.yaml) and makes them the baseline, so the next Save does
// not treat them as this config's own edits.
func (c *Config) SetMachineSections(mc MachineConfig) {
	mc = cloneMachine(mc)
	c.LlamaServer = mc.LlamaServer
	c.MistralRS = mc.MistralRS
	c.machineBase = newBaseline(mc)
}

// RefreshMachineSections takes mc (freshly read from machine.yaml) as the
// baseline and as the value of every runtime field this config has not
// changed since its own baseline. Fields with an unsaved local edit keep it,
// so the next Save still writes them: a refresh triggered by this config's
// own earlier save must not revert a newer edit. With no unsaved edits it is
// SetMachineSections.
func (c *Config) RefreshMachineSections(mc MachineConfig) {
	base := c.machineBase.get()
	cur := c.MachineSections()
	next := cloneMachine(mc)
	mergeChangedFields(&next.LlamaServer, &cur.LlamaServer, &base.LlamaServer)
	mergeChangedFields(&next.MistralRS, &cur.MistralRS, &base.MistralRS)
	c.LlamaServer = next.LlamaServer
	c.MistralRS = next.MistralRS
	c.machineBase = newBaseline(mc)
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
	base := c.machineBase.get()
	if reflect.DeepEqual(cur, base) {
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
	mergeChangedFields(&merged.LlamaServer, &cur.LlamaServer, &base.LlamaServer)
	mergeChangedFields(&merged.MistralRS, &cur.MistralRS, &base.MistralRS)
	return SaveMachine(merged)
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

// MachineSections returns a copy of the machine-wide runtime sections.
func (c Config) MachineSections() MachineConfig {
	return cloneMachine(MachineConfig{LlamaServer: c.LlamaServer, MistralRS: c.MistralRS})
}

// MarkMachineSaved advances the baseline to saved, the sections a successful
// Save of a copy of c just wrote, so later saves of c do not re-send them over
// another profile's newer values. It does nothing and returns false unless c's
// sections still equal saved: a newer edit or a refresh happened since the
// copy was taken and must not be lost.
func (c *Config) MarkMachineSaved(saved MachineConfig) bool {
	if !c.MachineSectionsEqual(saved) {
		return false
	}
	c.machineBase = newBaseline(saved)
	return true
}
