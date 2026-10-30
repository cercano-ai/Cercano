package routinglog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateWriterIsolation(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	path := filepath.Join(dir, "turn-routing.jsonl")
	for i := 0; i < 2; i++ {
		w, err := NewPrivateWriter(path)
		if err != nil {
			t.Fatal(err)
		}
		w.Log("private_test", nil)
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Count(string(data), "private_test") != 2 {
		t.Fatalf("log=%q err=%v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private mode: %v %v", info, err)
	}
	for _, invalid := range []string{"", filepath.Join(dir, "absent", "routing")} {
		if w, err := NewPrivateWriter(invalid); err == nil {
			w.Close()
			t.Fatalf("accepted %q", invalid)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "absent")); !os.IsNotExist(err) {
		t.Fatalf("created parent: %v", err)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("HOME touched: %v %v", entries, err)
	}
	w, err := NewWriter("")
	if err != nil {
		t.Fatal(err)
	}
	w.Log("legacy_test", nil)
	w.Close()
	data, err = os.ReadFile(filepath.Join(home, ".config", "cercano", "turn-routing.jsonl"))
	if err != nil || !strings.Contains(string(data), "legacy_test") {
		t.Fatalf("legacy default: %q %v", data, err)
	}
}
