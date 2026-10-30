package ui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDevHostSelectionRequiresExactIdentity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "selection.json")
	for _, input := range []string{`{}`, `{"Identity":{"host":"A"}}`, `not-json`} {
		if err := os.WriteFile(p, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadDevHostSelection(p); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	if _, err := (DevHostSelection{}).Dial(context.Background()); err == nil {
		t.Fatal("empty selection accepted")
	}
}

// Exercise the read/update/encode path used for returned management selections,
// without dialing a host or starting the CLI.
func TestDevHostSelectionPreservesConfigAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selection.json")
	authority := filepath.Join(t.TempDir(), "shared", "config.yaml")
	input := map[string]any{
		"ConfigAuthority": authority,
		"Identity": map[string]any{"host": "A", "build": "B", "contract": "C", "schema": 2, "store": "/isolated/conversations.db"},
		"Conversation": "conversation", "Epoch": "old-epoch",
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	selection, err := ReadDevHostSelection(path)
	if err != nil {
		t.Fatal(err)
	}
	selection.Epoch = "new-epoch"
	raw, err = json.Marshal(selection)
	if err != nil {
		t.Fatal(err)
	}
	var emitted map[string]any
	if err := json.Unmarshal(raw, &emitted); err != nil {
		t.Fatal(err)
	}
	if emitted["ConfigAuthority"] != authority || emitted["Epoch"] != "new-epoch" {
		t.Fatalf("updated selection lost authority or epoch: %s", raw)
	}
}
