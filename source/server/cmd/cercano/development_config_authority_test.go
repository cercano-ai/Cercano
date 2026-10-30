package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentConfigAuthorityIsolation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "state")
	shared := filepath.Join(state, "shared")
	if err := os.MkdirAll(shared, 0700); err != nil {
		t.Fatal(err)
	}
	selected := filepath.Join(shared, "config.yaml")
	if err := os.WriteFile(selected, []byte("locus_mode: cloud_only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := developmentConfigAuthority(state, selected)
	if err != nil || got != selected {
		t.Fatalf("authority %q: %v", got, err)
	}
	outside := filepath.Join(root, "outside.yaml")
	if err := os.WriteFile(outside, nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(shared, "alias.yaml")
	if err := os.Symlink(selected, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{outside, alias, "relative.yaml", filepath.Join(shared, "missing.yaml")} {
		if _, err := developmentConfigAuthority(state, path); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
	if err := os.Chmod(selected, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := developmentConfigAuthority(state, selected); err == nil {
		t.Fatal("accepted public config")
	}
}

func TestDevelopmentHostCommandConfigAuthority(t *testing.T) {
	// Inspect argv only: no process, native backend, network or application startup.
	cmd := developmentHostCommand("fake-binary", "fake-state", "fake-generation", "fake-host", "fake-token", "fake-record", "fake-runtime", io.Discard, "/synthetic/state/shared/config.yaml")
	n := len(cmd.Args)
	if cmd.Args[n-2] != "--config-authority" || cmd.Args[n-1] != "/synthetic/state/shared/config.yaml" {
		t.Fatalf("argv: %v", cmd.Args)
	}
}

func TestDevelopmentSelectionConfigAuthorityReuse(t *testing.T) {
	const authority = "/synthetic/state/shared/config.yaml"
	for _, recorded := range []string{"", "/synthetic/state/other/config.yaml", authority} {
		err := (launchSelection{ConfigAuthority: recorded}).validateConfigAuthority(authority)
		if (err == nil) != (recorded == authority) {
			t.Fatalf("recorded %q: %v", recorded, err)
		}
	}
}
