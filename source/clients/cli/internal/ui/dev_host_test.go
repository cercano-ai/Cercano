package ui

import (
	"context"
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
