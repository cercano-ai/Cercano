package testenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) { os.Exit(IsolateHome(m)) }

func TestIsolateHomeRedirectsHome(t *testing.T) {
	h := Home()
	if h == "" || !strings.HasPrefix(filepath.Base(h), "cercano-test-home-") {
		t.Fatalf("Home() = %q, want a temporary cercano-test-home- dir", h)
	}
	got, err := os.UserHomeDir()
	if err != nil || got != h {
		t.Fatalf("os.UserHomeDir() = %q, %v; want %q", got, err, h)
	}
	if os.Getenv("XDG_CONFIG_HOME") != filepath.Join(h, "xdg") {
		t.Errorf("XDG_CONFIG_HOME = %q", os.Getenv("XDG_CONFIG_HOME"))
	}
	for _, k := range pathOverrides {
		if _, set := os.LookupEnv(k); set {
			t.Errorf("%s still set", k)
		}
	}
}
