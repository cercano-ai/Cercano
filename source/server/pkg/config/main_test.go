package config

import (
	"os"
	"testing"
)

// TestMain keeps tests off the real home directory: Save writes machine.yaml
// under it.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "cercano-config-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
