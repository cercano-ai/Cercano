package mcphost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/agenttools"
)

// mcp.yaml lives in the profile directory; creating it keeps that private.
func TestPersistCreatesPrivateConfigDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profiles", "work")
	m := New(agenttools.NewRegistry(), dir, time.Second)
	if err := m.Remove(context.Background(), "absent"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("config dir mode = %v, want 0700", fi.Mode().Perm())
	}
}
