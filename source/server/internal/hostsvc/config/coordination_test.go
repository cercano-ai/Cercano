package config

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	cfg "cercano/source/server/pkg/config"
)

// Reproduces the legacy two-store lost update. Keep this passing characterization
// until callers explicitly adopt coordinated persistence; do not change Persist.
func TestLegacyTwoStoreLostUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	initial := cfg.Defaults()
	if err := cfg.Save(initial, path); err != nil {
		t.Fatal(err)
	}
	a, b := New(path, initial, nil), New(path, initial, nil)
	if err := a.Mutate(func(c *cfg.Config) { c.Port = "12345" }); err != nil {
		t.Fatal(err)
	}
	if err := b.Mutate(func(c *cfg.Config) { c.OllamaURL = "http://fixture.invalid" }); err != nil {
		t.Fatal(err)
	}
	if err := a.Persist(); err != nil {
		t.Fatal(err)
	}
	if err := b.Persist(); err != nil {
		t.Fatal(err)
	}
	got, err := cfg.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Port == "12345" {
		t.Fatal("legacy stale Persist unexpectedly preserved first edit")
	}
	if got.OllamaURL != "http://fixture.invalid" {
		t.Fatal("second edit missing")
	}
}

func TestPreparedCoordinationDoesNotActivateService(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	initial := cfg.Defaults()
	if err := cfg.Save(initial, path); err != nil {
		t.Fatal(err)
	}
	a, b := New(path, initial, nil), New(path, initial, nil)
	ca, err := PrepareDiskCoordination(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := PrepareDiskCoordination(b)
	if err != nil {
		t.Fatal(err)
	}
	base, err := ca.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ca.Change(context.Background(), base, cfg.ConfigChange{Path: []string{"port"}, YAML: []byte("'12345'")}); err != nil {
		t.Fatal(err)
	}
	if _, err := cb.Change(context.Background(), base, cfg.ConfigChange{Path: []string{"ollama_url"}, YAML: []byte("http://fixture.invalid")}); err != nil {
		t.Fatal(err)
	}
	if a.Get().Port != initial.Port || b.Get().OllamaURL != initial.OllamaURL {
		t.Fatal("disk publication activated runtime state")
	}
	if _, err := PrepareDiskCoordination(New(path, initial, nil, Options{Frozen: true})); !errors.Is(err, ErrFrozen) {
		t.Fatalf("frozen: %v", err)
	}
}
