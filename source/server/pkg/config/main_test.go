package config

import (
	"os"
	"testing"
)

// envOverrideVars lists every variable applyEnvOverrides reads. Keep in sync.
var envOverrideVars = []string{
	"OLLAMA_URL",
	"CERCANO_LOCAL_RUNTIME",
	"CERCANO_OPEN_RUNTIME",
	"CERCANO_LOCAL_MODEL",
	"CERCANO_OPEN_MODEL",
	"CERCANO_EMBEDDING_MODEL",
	"CERCANO_PORT",
	"GEMINI_API_KEY",
}

// TestMain clears the developer's environment overrides so Load sees only
// what each test sets. Without this, an exported GEMINI_API_KEY (common on a
// dev machine) injects legacy cloud fields into every Load and fails tests
// that assert those fields stay empty. Tests that exercise an override set it
// explicitly with t.Setenv. It also points HOME at a temp dir because Save
// writes machine.yaml under it.
func TestMain(m *testing.M) {
	for _, name := range envOverrideVars {
		os.Unsetenv(name)
	}
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
