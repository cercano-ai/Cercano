package config

import (
	"cercano/source/server/internal/secrets"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlySaveBeforeFilesystemEffects(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	cfg, err := LoadWithOptions("", secrets.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []Config{cfg, cfg.Clone(), Defaults().WithSharedReadOnly().Clone()} {
		root := t.TempDir()
		path := filepath.Join(root, "must-not-create", "config.yaml")
		if err := Save(c, path); !errors.Is(err, secrets.ErrReadOnly) {
			t.Fatalf("Save: %v", err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			t.Fatalf("file effects: %v %v", entries, err)
		}
	}
}
