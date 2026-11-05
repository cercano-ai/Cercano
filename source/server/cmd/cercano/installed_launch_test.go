package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/conversation"
)

// An installed agent must not need the independently installed CLI or a source
// publisher manifest. Store preparation remains a separate prerequisite.
func TestInstalledLaunchLayoutProbe(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "il-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "state")
	rt := filepath.Join(root, "rt")
	for _, p := range []string{state, rt} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := conversation.PrepareDevelopmentStore(context.Background(), filepath.Join(state, "conversations.db")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "config.yaml"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	// This test executable is deliberately not installed. Installed launch must
	// reach executable-layout validation, not demand source-generation flags.
	err = runDevelopmentLaunch(context.Background(), []string{"--installed", "--state", state, "--runtime-root", rt}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "installed agent requires") {
		t.Fatalf("installed-layout adapter missing: %v", err)
	}
}
