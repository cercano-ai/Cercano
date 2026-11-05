package main

import (
	"context"
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
