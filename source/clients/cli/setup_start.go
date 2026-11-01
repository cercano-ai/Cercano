package main

import (
	"cercano/source/clients/cli/internal/wizard"
	"os"
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
	return needsSetup(explicit, authority)
}
