package main

import (
	"fmt"
	"os"
	"path/filepath"

	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
)

// profileFile returns name beside the active profile's config.yaml, where
// every profile-owned file (crash.log, telemetry.db, permissions.yaml,
// catalog-cache.json, mcp.yaml) lives.
func profileFile(name string) string {
	return filepath.Join(filepath.Dir(config.DefaultPath()), name)
}

// dispatchLogPath is the dispatch diagnostics log: logs/dispatch.log in the
// active profile's directory.
func dispatchLogPath() string {
	return filepath.Join(paths.ActiveProfileDir(), "logs", "dispatch.log")
}

// updateCacheDir holds update_check.json. A release check is about the
// installed binary, not a profile, so the cache is machine-wide.
func updateCacheDir() string { return paths.ConfigRoot() }

// checkStartupEnvironment fails when profile and machine paths cannot be
// resolved: no home directory, or an invalid CERCANO_PROFILE. Running anyway
// would scatter data into the working directory or quietly use the default
// profile's data.
func checkStartupEnvironment() error {
	if err := paths.CheckHome(); err != nil {
		return err
	}
	if _, err := paths.Profile(); err != nil {
		return fmt.Errorf("CERCANO_PROFILE: %w", err)
	}
	return nil
}

// needsStartupCheck reports whether the command in args reads or writes
// profile or machine state. Only commands that print static information or
// write into an explicit directory skip the check.
func needsStartupCheck(args []string) bool {
	if len(args) < 2 {
		return true
	}
	switch args[1] {
	case "version", "-version", "--version", "protocols", "skills":
		return false
	}
	return true
}

// requireStartupEnvironment exits with a readable error when
// checkStartupEnvironment fails.
func requireStartupEnvironment(args []string) {
	if !needsStartupCheck(args) {
		return
	}
	if err := checkStartupEnvironment(); err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}
}
