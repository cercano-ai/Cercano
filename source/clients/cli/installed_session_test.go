package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstalledSessionSelection(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(key, root)
	}
	t.Setenv("PATH", root)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := selectInstalledSession(ctx, "relative", root); err == nil {
		t.Fatal("relative state accepted")
	}
	if _, err := selectInstalledSession(ctx, root, root); err == nil {
		t.Fatal("missing agent accepted")
	}
	selection := filepath.Join(root, "session.json")
	agent := filepath.Join(root, "cercano")
	script := "#!/bin/sh\n[ \"$1\" = development-launch ] && [ \"$2\" = --installed ] || exit 42\nprintf '%s\\n' '" + selection + "'\n"
	if err := os.WriteFile(agent, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`{"Identity":{"Host":"A","Build":"build-A","Contract":"c","Schema":1,"Store":"store"},"Conversation":"conversation","Epoch":"epoch"}`, true},
		{`{"Fixture":true,"Identity":{"Host":"A","Build":"build-A","Contract":"c","Schema":1,"Store":"store"}}`, false},
		{`{}`, false},
	} {
		if err := os.WriteFile(selection, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		path, err := selectInstalledSession(ctx, root, root)
		if (err == nil) != tc.ok || (tc.ok && path != selection) {
			t.Fatalf("selection %s: %q %v", tc.body, path, err)
		}
	}
}

func TestInstalledSessionDefaultDiscovery(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "unused-xdg"))
	for _, key := range []string{"CERCANO_DEVELOPMENT_STATE", "CERCANO_RUNTIME_ROOT", "CERCANO_CONVERSATIONS_DB", "CERCANO_DEVELOPMENT_CONFIG_AUTHORITY"} {
		t.Setenv(key, "")
	}
	state := filepath.Join(root, ".config", "cercano")
	if err := os.MkdirAll(state, 0700); err != nil {
		t.Fatal(err)
	}
	runtime := filepath.Join(root, "runtime")
	record, _ := json.Marshal(runtime)
	if err := os.WriteFile(filepath.Join(state, "runtime-root.json"), record, 0600); err != nil {
		t.Fatal(err)
	}
	selection := filepath.Join(root, "selection.json")
	if err := os.WriteFile(selection, []byte(`{"Identity":{"Host":"A","Build":"build-A","Contract":"c","Schema":1,"Store":"store"},"Conversation":"conversation","Epoch":"epoch"}`), 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n[ \"$4\" = '" + state + "' ] && [ \"$6\" = '" + runtime + "' ] || exit 42\nprintf '%s\\n' '" + selection + "'\n"
	if err := os.WriteFile(filepath.Join(root, "cercano"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if path, err := selectInstalledSession(ctx, "", ""); err != nil || path != selection {
		t.Fatalf("default existing-state selection: %q %v", path, err)
	}
}
