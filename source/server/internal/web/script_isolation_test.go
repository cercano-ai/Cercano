package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchScriptPrivateOverride(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(t.TempDir(), "scripts")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CERCANO_SEARCH_SCRIPT_DIR", dir)
	got, err := EnsureSearchScript("")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "ddg_search.py") {
		t.Fatalf("path = %q; want private directory %q", got, dir)
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
		t.Fatalf("HOME config touched: %v", err)
	}
}

func TestSearchScriptLegacyDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CERCANO_SEARCH_SCRIPT_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	got, err := EnsureSearchScript("")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, ".config", "cercano", "scripts", "ddg_search.py") {
		t.Fatalf("unexpected default: %q", got)
	}
}

func TestSearchScriptExplicitDirectoryWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CERCANO_SEARCH_SCRIPT_DIR", t.TempDir())
	dir := t.TempDir()
	got, err := EnsureSearchScript(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "ddg_search.py") {
		t.Fatalf("explicit path ignored: %q", got)
	}
}

func TestSearchScriptRejectsRelativeOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CERCANO_SEARCH_SCRIPT_DIR", "relative")
	if _, err := EnsureSearchScript(""); err == nil {
		t.Fatal("relative override accepted")
	}
}
