package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/secrets"
)

// Exercise the exact command used by the launcher, but execute only a fake
// helper: no startup, state provisioning, credential backend, or provider.
func TestDevelopmentHostCommandCredentialPolicy(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(key, root)
	}
	helper := filepath.Join(root, "fake-host")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\n[ \"$1\" = development-host ] || exit 2\nprintf '%s:%s' \"${CERCANO_DISABLE_CREDENTIALS+x}\" \"${CERCANO_DISABLE_CREDENTIALS-}\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, value             string
		present, allowed, valid bool
	}{
		{"absent", "", false, true, true},
		{"enabled", "0", true, true, true},
		{"disabled", "1", true, false, true},
		{"empty", "", true, false, false},
		{"malformed", "true", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(secrets.DisableEnv, tc.value)
			if !tc.present {
				if err := os.Unsetenv(secrets.DisableEnv); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			cmd := developmentHostCommand(helper, "fake-state", "fake-generation", "fake-host", "fake-token", "fake-record", "fake-runtime", &output)
			if err := cmd.Run(); err != nil {
				t.Fatalf("fake child: %v: %s", err, &output)
			}
			want := ":"
			if tc.present {
				want = "x:" + tc.value
			}
			if got := output.String(); got != want {
				t.Errorf("child policy = %q, want %q", got, want)
			}
			// Pure policy checks only; these never open a credential backend.
			if valid := secrets.ValidateEnvironment() == nil; valid != tc.valid {
				t.Errorf("valid = %v, want %v", valid, tc.valid)
			}
			if allowed := secrets.CheckAllowed() == nil; allowed != tc.allowed {
				t.Errorf("allowed = %v, want %v", allowed, tc.allowed)
			}
		})
	}
}
