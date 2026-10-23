//go:build unix

package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCredentialDisabledWorkerInheritance(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("TMPDIR", dir)
	t.Setenv("PATH", dir)
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	output := filepath.Join(dir, "env")
	t.Setenv("CERCANO_FIXTURE_OUTPUT", output)
	script := "#!/bin/sh\nprintf '%s' \"$CERCANO_DISABLE_CREDENTIALS\" > \"$CERCANO_FIXTURE_OUTPUT\"\n"
	if err := os.WriteFile(filepath.Join(dir, "cercano"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	// Fixture intentionally opens no socket. Startup times out and reaps it.
	h, err := spawnWorker(ctx, "env-fixture", 1)
	if h != nil {
		h.Kill()
	}
	if err == nil {
		t.Fatal("fixture unexpectedly served")
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "1" {
		t.Fatalf("inheritance: %q %v", data, err)
	}
}
