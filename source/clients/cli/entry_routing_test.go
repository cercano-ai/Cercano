package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAbsentInstalledEntryPreservesLegacy(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CERCANO_DEVELOPMENT_STATE", root)
	t.Setenv("CERCANO_RUNTIME_ROOT", "")
	got, err := discoverInstalledEntry()
	if err != nil || got {
		t.Fatalf("absent runtime record must retain legacy routing: installed=%v err=%v", got, err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime-root.json"), []byte(`not-json`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = discoverInstalledEntry()
	if err == nil && !got {
		t.Fatal("discovered malformed record must not fall back")
	}
}
