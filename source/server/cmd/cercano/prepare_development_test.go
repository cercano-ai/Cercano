package main

import (
	"bytes"
	"cercano/source/server/pkg/config"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cercano/source/server/internal/conversation"
)

func TestPrepareDevelopmentExclusive(t *testing.T) {
	prepareConfigFixture(t)
	ctx := context.Background()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(parent, "legacy.db")
	original := []byte("legacy sentinel")
	if err := os.WriteFile(legacy, original, 0600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "new")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- runPrepareDevelopment(ctx, []string{"--new-state", root}, &bytes.Buffer{})
		}()
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful exclusive creators: %d", successes)
	}
	db := filepath.Join(root, "conversations.db")
	if err := conversation.PreflightCoexistence(ctx, db, conversation.DevelopmentCoexistenceDeclaration()); err != nil {
		t.Fatal(err)
	}
	if err := conversation.PreflightCoexistence(ctx, db, conversation.CurrentCoexistenceDeclaration()); err == nil {
		t.Fatal("development accepted as fixture")
	}
	store, err := conversation.OpenCoexistence(ctx, db, conversation.DevelopmentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	if err := conversation.PrepareDevelopmentStore(ctx, db); err == nil {
		t.Fatal("overwrote prepared DB")
	}
	got, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("legacy changed: %q %v", got, err)
	}
}

func TestPrepareDevelopmentRefusesExisting(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"directory", "file", "link"} {
		path := filepath.Join(parent, name)
		switch name {
		case "directory":
			err = os.Mkdir(path, 0700)
		case "file":
			err = os.WriteFile(path, nil, 0600)
		case "link":
			err = os.Symlink(filepath.Join(parent, "directory"), path)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := runPrepareDevelopment(context.Background(), []string{"--new-state", path}, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted existing %s", name)
		}
	}
	if err := runPrepareDevelopment(context.Background(), []string{"--new-state", "relative"}, &bytes.Buffer{}); err == nil {
		t.Fatal("accepted relative destination")
	}
}

// All onboarding inputs are synthetic; never consult the operator's home.
func prepareConfigFixture(t *testing.T) ([]byte, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	path := config.DefaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte("# synthetic settings\nlocus_mode: cloud_only\nactive_cloud_profile: fixture\ncloud_api_key: synthetic-inline-secret\ncloud_profiles:\n  - name: fixture\n    credential_ref: synthetic-reference\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return data, path
}

func TestPrepareDevelopmentCarriesConfig(t *testing.T) {
	original, source := prepareConfigFixture(t)
	legacy := filepath.Join(filepath.Dir(source), "conversations.db")
	sentinel := []byte("synthetic legacy database -- never import")
	if err := os.WriteFile(legacy, sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "development")
	var output bytes.Buffer
	if err := runPrepareDevelopment(context.Background(), []string{"--new-state", root}, &output); err != nil {
		t.Fatal(err)
	}
	legacyAfter, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(legacyAfter, sentinel) {
		t.Fatal("legacy source mutated")
	}
	if err := conversation.PreflightCoexistence(context.Background(), filepath.Join(root, "conversations.db"), conversation.DevelopmentCoexistenceDeclaration()); err != nil {
		t.Fatal("fresh development database required:", err)
	}
	destination := filepath.Join(root, "config.yaml")
	copied, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(copied, original) {
		t.Fatal("config bytes not preserved")
	}
	for path, mode := range map[string]os.FileMode{root: 0700, destination: 0600} {
		st, err := os.Stat(path)
		if err != nil || st.Mode().Perm() != mode {
			t.Fatalf("wrong private mode: %s", path)
		}
	}
	if strings.Contains(output.String(), "synthetic-inline-secret") {
		t.Fatal("config leaked to output")
	}
	changed := []byte("# development settings independently edited\n")
	if err := os.WriteFile(destination, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := runPrepareDevelopment(context.Background(), []string{"--new-state", root}, &output); err == nil {
		t.Fatal("accepted existing destination")
	}
	got, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(got, changed) {
		t.Fatal("destination overwritten")
	}
	got, err = os.ReadFile(source)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatal("original mutated")
	}
	st, _ := os.Stat(source)
	if st.Mode().Perm() != 0644 {
		t.Fatal("original permissions mutated")
	}
}

func TestPrepareDevelopmentMissingConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "development")
	err = runPrepareDevelopment(context.Background(), []string{"--new-state", root}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "source config") {
		t.Fatalf("expected clear missing source config error, got %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("missing source left partial state")
	}
}
