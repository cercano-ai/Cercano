package main

import (
	"cercano/source/clients/cli/internal/wizard"
	"cercano/source/server/pkg/setupstate"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

// needsSetup preserves ordinary startup policy while allowing a reset to seed
// a fresh wizard without deleting configuration that contains user preferences.
func needsSetup(explicit bool, configPath string) bool {
	if explicit {
		return true
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return true
	}
	_, resume := wizard.Load()
	return resume
}

// A pinned host's explicit config path is authoritative; never probe the
// caller's default config when the host did not supply one.
func needsPinnedSetup(explicit bool, authority string) bool {
	if authority == "" {
		return explicit
	}
	if explicit {
		return true
	}
	if _, err := os.Stat(authority); os.IsNotExist(err) {
		return true
	}
	// Use the existing setupstate schema and adjacent wizard_state.yaml
	// convention, not the caller's ambient XDG/override resume file.
	data, err := os.ReadFile(filepath.Join(filepath.Dir(authority), "wizard_state.yaml"))
	if err != nil {
		return false
	}
	var state setupstate.State
	return yaml.Unmarshal(data, &state) == nil && state.Step != ""
}
