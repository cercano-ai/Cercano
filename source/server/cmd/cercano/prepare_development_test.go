package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"cercano/source/server/internal/conversation"
)

func TestPrepareDevelopmentExclusive(t *testing.T) {
	ctx := context.Background()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(parent, "legacy.db")
	original := []byte("legacy sentinel")
	if err := os.WriteFile(legacy, original, 0600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "new")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- runPrepareDevelopment(ctx, []string{"--new-state", root}, &bytes.Buffer{})
		}()
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful exclusive creators: %d", successes)
	}
	db := filepath.Join(root, "conversations.db")
	if err := conversation.PreflightCoexistence(ctx, db, conversation.DevelopmentCoexistenceDeclaration()); err != nil {
		t.Fatal(err)
	}
	if err := conversation.PreflightCoexistence(ctx, db, conversation.CurrentCoexistenceDeclaration()); err == nil {
		t.Fatal("development accepted as fixture")
	}
	store, err := conversation.OpenCoexistence(ctx, db, conversation.DevelopmentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	if err := conversation.PrepareDevelopmentStore(ctx, db); err == nil {
		t.Fatal("overwrote prepared DB")
	}
	got, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("legacy changed: %q %v", got, err)
	}
}

func TestPrepareDevelopmentRefusesExisting(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"directory", "file", "link"} {
		path := filepath.Join(parent, name)
		switch name {
		case "directory":
			err = os.Mkdir(path, 0700)
		case "file":
			err = os.WriteFile(path, nil, 0600)
		case "link":
			err = os.Symlink(filepath.Join(parent, "directory"), path)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := runPrepareDevelopment(context.Background(), []string{"--new-state", path}, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted existing %s", name)
		}
	}
	if err := runPrepareDevelopment(context.Background(), []string{"--new-state", "relative"}, &bytes.Buffer{}); err == nil {
		t.Fatal("accepted relative destination")
	}
}
