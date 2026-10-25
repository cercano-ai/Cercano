// Package testenv keeps tests away from the developer's real Cercano data.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// pathOverrides are environment variables that point a default Cercano path
// somewhere explicit, or pick the profile. A developer's shell may set them
// to real locations, so isolated tests clear them.
var pathOverrides = []string{
	"CERCANO_PROFILE",
	"CERCANO_CONVERSATIONS_DB",
	"CERCANO_WIZARD_STATE",
	"CERCANO_ANOMALY_LOG",
}

var home string

// IsolateHome points HOME, USERPROFILE and XDG_CONFIG_HOME at a fresh
// temporary directory and clears the path overrides, runs the tests, then
// removes the directory. Every package whose tests can reach a default
// Cercano path (config.Save, NewServer, the profile logs, machine.yaml, the
// model store) calls it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testenv.IsolateHome(m)) }
func IsolateHome(m *testing.M) int {
	dir, err := os.MkdirTemp("", "cercano-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testenv: create temporary home:", err)
		return 1
	}
	defer os.RemoveAll(dir)
	home = dir
	for k, v := range map[string]string{"HOME": dir, "USERPROFILE": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "xdg")} {
		if err := os.Setenv(k, v); err != nil {
			fmt.Fprintln(os.Stderr, "testenv:", err)
			return 1
		}
	}
	for _, k := range pathOverrides {
		os.Unsetenv(k)
	}
	// Isolated tests exercise the per-profile agent-host machinery (profile
	// dialing, host registries, per-profile restarts), which is opt-in at
	// runtime: turn it on here so the default stays off in production.
	// Tests of the off path unset or overwrite it themselves.
	if err := os.Setenv("CERCANO_PER_PROFILE_AGENTS", "1"); err != nil {
		fmt.Fprintln(os.Stderr, "testenv:", err)
		return 1
	}
	return m.Run()
}

// Home returns the temporary home IsolateHome set, or "" when the package's
// tests did not call it.
func Home() string { return home }
