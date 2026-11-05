package main

import (
	"fmt"
	"os"
	"path/filepath"

	"cercano/source/server/pkg/config"
)

// Absence alone preserves singleton startup. A discovered record (even an
// ineligible one) is admitted by selectInstalledSession, never legacy fallback.
func discoverInstalledEntry() (bool, error) {
	if os.Getenv("CERCANO_RUNTIME_ROOT") != "" {
		return true, nil
	}
	state := os.Getenv("CERCANO_DEVELOPMENT_STATE")
	if state == "" {
		state = filepath.Dir(config.DefaultPath())
	}
	_, err := os.Lstat(filepath.Join(state, "runtime-root.json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect prepared runtime record: %w", err)
	}
	return true, nil
}
