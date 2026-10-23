package config

import (
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

// overlayMachine replaces the runtime sections of cfg with machine.yaml's.
// When machine.yaml does not exist, cfg keeps whatever the profile file held
// (an older version wrote these sections there) so nothing is lost before
// migration.
func overlayMachine(cfg *Config) error {
	if !machineFileExists() {
		return nil
	}
	mc, err := LoadMachine()
	if err != nil {
		return err
	}
	cfg.LlamaServer = mc.LlamaServer
	cfg.MistralRS = mc.MistralRS
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
