package config

import (
	"cercano/source/server/internal/secrets"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialDisabledConfigWriteRefused(t *testing.T) {
	t.Setenv(secrets.DisableEnv, "1")
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("cloud_api_key: original\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	cfg.CloudAPIKey = "inline"
	if err := Save(cfg, path); !errors.Is(err, secrets.ErrDisabled) {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(original) {
		t.Fatalf("config modified: %q %v", got, err)
	}
}

func TestMalformedCredentialSettingFailsBeforeConfigRead(t *testing.T) {
	t.Setenv(secrets.DisableEnv, "yes")
	if _, err := Load(filepath.Join(t.TempDir(), "absent")); !errors.Is(err, secrets.ErrDisabled) {
		t.Fatal(err)
	}
}

func TestDisabledConfigPreservesNonsecretOverrides(t *testing.T) {
	t.Setenv(secrets.DisableEnv, "1")
	t.Setenv("GEMINI_API_KEY", "fixture-not-a-real-key")
	t.Setenv("CERCANO_PORT", "12345")
	cfg, err := Load(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CloudAPIKey != "" || cfg.Port != "12345" {
		t.Fatal("credential imported or nonsecret override lost")
	}
	if err := Save(cfg, filepath.Join(t.TempDir(), "safe.yaml")); err != nil {
		t.Fatal(err)
	}
}
