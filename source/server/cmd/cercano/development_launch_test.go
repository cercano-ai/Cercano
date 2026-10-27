package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevelopmentLaunchRefusesBeforeSpawn(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, runtime, want string }{
		{"missing", filepath.Join(root, "missing"), "no such file"},
		{"long", root, "runtime-root exceeds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runDevelopmentLaunch(context.Background(), []string{"--state", root, "--generation", root, "--runtime-root", tc.runtime}, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatal("refusal mutated state")
			}
		})
	}
}
func TestDevelopmentSelectionNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "A.json")
	if e := launchExclusive(path, map[string]string{"Conversation": "A"}); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(path)
	if e := launchExclusive(path, map[string]string{"Conversation": "B"}); !os.IsExist(e) {
		t.Fatalf("overwrite: %v", e)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("selection rewritten")
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal("selection not private")
	}
}
func TestDevelopmentPrivatePaths(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	os.Chmod(root, 0700)
	if e := launchPrivateDir(root); e != nil {
		t.Fatal(e)
	}
	alias := root + "-alias"
	if e := os.Symlink(root, alias); e != nil {
		t.Fatal(e)
	}
	defer os.Remove(alias)
	if launchPrivateDir(alias) == nil {
		t.Fatal("alias accepted")
	}
	os.Chmod(root, 0755)
	if launchPrivateDir(root) == nil {
		t.Fatal("public directory accepted")
	}
}
