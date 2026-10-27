package conversation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentStoreRefusesLegacyAndAliases(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "legacy.db")
	s, err := Open(legacy)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	before, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := PreflightCoexistence(ctx, legacy, DevelopmentCoexistenceDeclaration()); err == nil {
		t.Fatal("adopted legacy")
	}
	if err := PrepareDevelopmentStore(ctx, legacy); err == nil {
		t.Fatal("overwrote legacy")
	}
	after, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("legacy database modified")
	}
	alias := filepath.Join(root, "alias.db")
	if err := os.Symlink(legacy, alias); err != nil {
		t.Fatal(err)
	}
	if err := PrepareDevelopmentStore(ctx, alias); err == nil {
		t.Fatal("accepted symlink")
	}
	empty := filepath.Join(root, "empty.db")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := PrepareDevelopmentStore(ctx, empty); err == nil {
		t.Fatal("adopted empty file")
	}
	public := filepath.Join(root, "public")
	if err := os.Mkdir(public, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(public, 0755); err != nil {
		t.Fatal(err)
	}
	if err := PrepareDevelopmentStore(ctx, filepath.Join(public, "new.db")); err == nil {
		t.Fatal("accepted nonprivate parent")
	}
	fixture := filepath.Join(root, "fixture.db")
	if err := PrepareCoexistence(ctx, fixture); err != nil {
		t.Fatal(err)
	}
	if err := PreflightCoexistence(ctx, fixture, DevelopmentCoexistenceDeclaration()); err == nil {
		t.Fatal("fixture accepted as development")
	}
	if err := PreflightCoexistence(ctx, fixture, CurrentCoexistenceDeclaration()); err != nil {
		t.Fatal(err)
	}
}
