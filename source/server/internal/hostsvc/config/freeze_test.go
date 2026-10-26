package config

import (
	"cercano/source/server/internal/credentialtest"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"cercano/source/server/internal/secrets"
	cfg "cercano/source/server/pkg/config"
)

func TestFrozenConfigBoundary(t *testing.T) {
	credentialtest.Enable(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("untouched snapshot\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	initial := cfg.Config{CloudModel: "original", ActiveCloudProfile: "a", CloudProfiles: []cfg.CloudProfile{{Name: "a", Model: "one"}, {Name: "b", Model: "two"}}}
	options := Options{Frozen: true}
	store := secrets.NewMemory()
	service := New(path, initial, store, options)
	options.Frozen = false // caller cannot thaw the service
	initial.CloudProfiles[0].Model = "caller changed"
	before := service.Get()
	if before.CloudProfiles[0].Model != "one" {
		t.Fatal("constructor did not isolate snapshot")
	}
	attempt, err := service.Credentials().BeginLogin(context.Background(), "a", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	defer attempt.Close()
	cases := []struct {
		name  string
		write func() error
	}{
		{"Set", func() error { return service.Set(cfg.Config{}) }},
		{"Mutate", func() error { return service.Mutate(func(*cfg.Config) { t.Fatal("frozen mutation callback invoked") }) }},
		{"UpsertProfile", func() error {
			_, _, err := service.UpsertProfile(cfg.CloudProfile{Name: "a", Model: "changed"})
			return err
		}},
		{"SetCloudModel", func() error { return service.SetCloudModel("changed") }},
		{"SetDestinationProfiles", func() error { return service.SetDestinationProfiles(cfg.DestinationPrimary, "b", "") }},
		{"SetTaskAssignment", func() error { return service.SetTaskAssignment(cfg.TaskChat, &cfg.TaskAssignment{}) }},
		{"SetPath", func() error { return service.SetPath("elsewhere") }},
		{"SetSecrets", func() error { return service.SetSecrets(secrets.NewMemory()) }},
		{"Persist", service.Persist},
		{"BeginCloudLogin", func() error {
			_, err := service.BeginCloudLogin(context.Background(), cfg.CloudProfile{Name: "a"}, true, false)
			return err
		}},
		{"CommitCloudLogin", func() error { _, err := (&CloudLogin{owner: service.(*svc)}).Commit("unused"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.write(); !errors.Is(err, ErrFrozen) {
				t.Fatalf("got %v, want ErrFrozen", err)
			}
			if !reflect.DeepEqual(before, service.Get()) {
				t.Fatal("live snapshot changed")
			}
			if attempt.Context().Err() != nil {
				t.Fatal("login canceled")
			}
		})
	}
	if service.SetActiveProfile("b") || service.SetBackupProfile("b") {
		t.Fatal("frozen selection accepted")
	}
	if existed, active := service.RemoveProfile("a"); existed || active {
		t.Fatal("frozen removal accepted")
	}
	if !reflect.DeepEqual(before, service.Get()) {
		t.Fatal("partial setters changed live config")
	}
	if attempt.Context().Err() != nil {
		t.Fatal("partial setter canceled login")
	}
	if service.Path() != path {
		t.Fatal("path changed")
	}
	if exists, active := service.ProfileInfo("a"); !exists || !active {
		t.Fatal("profile read failed")
	}
	profile, ok := service.ActiveProfile()
	if !ok || profile.Model != "one" {
		t.Fatal("active read failed")
	}
	snapshot := service.Get()
	snapshot.CloudProfiles[0].Model = "changed reader"
	if service.Get().CloudProfiles[0].Model != "one" {
		t.Fatal("reader mutated live state")
	}
	if service.Secrets() == nil || service.Credentials() == nil {
		t.Fatal("store readers unavailable")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(original) {
		t.Fatalf("file changed: %q, %v", data, err)
	}
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if err := New(missing, before, nil, Options{Frozen: true}).Persist(); !errors.Is(err, ErrFrozen) {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("frozen Persist created a file: %v", err)
	}
}

func TestWritablePersistReturnsErrors(t *testing.T) {
	credentialtest.Enable(t)
	service := New(filepath.Join(t.TempDir(), "config.yaml"), cfg.Config{}, nil)
	if err := service.SetCloudModel("writable"); err != nil {
		t.Fatal(err)
	}
	if err := service.Persist(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(service.Path()); err != nil {
		t.Fatal(err)
	}
	if err := service.SetPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := service.Persist(); err == nil {
		t.Fatal("expected file write error")
	}
}
