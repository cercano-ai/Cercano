package config

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	cfg "cercano/source/server/pkg/config"
)

// Regression for the reproduced legacy two-store lost update.
func TestCoordinatedTwoStoreStalePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	initial, err := cfg.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	next, err := cfg.SaveSnapshot(initial, path, initial.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	initial = initial.WithSnapshot(next)
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
	if err := b.Persist(); !errors.Is(err, cfg.ErrConfigConflict) {
		t.Fatal(err)
	}
	got, err := cfg.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Port != "12345" {
		t.Fatal("stale Persist lost first edit")
	}
	if got.OllamaURL == "http://fixture.invalid" {
		t.Fatal("stale edit published")
	}
}

func TestPreparedCoordinationDoesNotActivateService(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	initial, err := cfg.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	next, err := cfg.SaveSnapshot(initial, path, initial.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	initial = initial.WithSnapshot(next)
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

// SaveAndSet, unlike legacy Mutate followed by Persist, admits the candidate
// before replacing live state and advances only the successful draft.
func TestSaveAndSetBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	initial, err := cfg.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	service := New(path, initial, nil)
	stale := service.Get()
	proposal := service.Get()
	proposal.Port = "12345"
	if err := service.SaveAndSet(proposal); err != nil {
		t.Fatal(err)
	}
	proposal = service.Get()
	proposal.Port = "23456"
	if err := service.SaveAndSet(proposal); err != nil {
		t.Fatal(err)
	}
	before := service.Get()
	stale.Port = "34567"
	if err := service.SaveAndSet(stale); !errors.Is(err, cfg.ErrConfigConflict) {
		t.Fatalf("stale: %v", err)
	}
	if !reflect.DeepEqual(before, service.Get()) {
		t.Fatal("rejected candidate changed live configuration")
	}
	if err := service.SaveAndSet(cfg.Defaults()); !errors.Is(err, cfg.ErrConfigConflict) {
		t.Fatalf("missing baseline: %v", err)
	}
	other := filepath.Join(t.TempDir(), "config.yaml")
	service.SetPath(other)
	if err := service.SaveAndSet(before); !errors.Is(err, cfg.ErrConfigConflict) {
		t.Fatalf("retarget: %v", err)
	}
	if !reflect.DeepEqual(before, service.Get()) {
		t.Fatal("failed admission changed live configuration")
	}
}
