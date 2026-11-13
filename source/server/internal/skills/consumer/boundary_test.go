package consumer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrontmatterBoundary(t *testing.T) {
	t.Run("two lines", func(t *testing.T) {
		if _, ok := extractFrontmatter("---\nname: x"); ok {
			t.Fatal("accepted missing delimiter")
		}
	})
	t.Run("junk delimiter", func(t *testing.T) {
		if _, ok := extractFrontmatter("---\nname: x\n---junk\nbody"); ok {
			t.Fatal("accepted junk delimiter")
		}
	})
}

func TestCatalogHardBudget(t *testing.T) {
	got := Catalog([]Entry{{Name: "x", Description: "d", Path: strings.Repeat("p", MaxCatalogBytes)}})
	if len(got) > MaxCatalogBytes {
		t.Fatalf("rendered %d bytes, limit %d", len(got), MaxCatalogBytes)
	}
}

func TestSymlinkedRootParent(t *testing.T) {
	work, external := t.TempDir(), t.TempDir()
	dir := filepath.Join(external, "skills", "example")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: example\ndescription: Example\n---\nbody"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(work, ".agents")); err != nil {
		t.Skip(err)
	}
	if got := Discover(work, ""); len(got) != 0 {
		t.Fatalf("followed parent symlink: %v", got)
	}
}

func TestSelectionSortedBeforeLimit(t *testing.T) {
	work := t.TempDir()
	for i := MaxSkills + 8; i >= 0; i-- {
		name := fmt.Sprintf("skill-%02d", i)
		dir := filepath.Join(work, ".agents", "skills", name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf("---\nname: %s\ndescription: Example\n---\nbody", name)
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got := Discover(work, "")
	if len(got) != MaxSkills {
		t.Fatalf("got %d entries", len(got))
	}
	for i, e := range got {
		if want := fmt.Sprintf("skill-%02d", i); e.Name != want {
			t.Fatalf("entry %d: got %q want %q", i, e.Name, want)
		}
	}
}

func TestCatalogRenderedBudget(t *testing.T) {
	entries := make([]Entry, MaxSkills)
	for i := range entries {
		entries[i] = Entry{Name: fmt.Sprintf("skill-%02d", i), Description: strings.Repeat("x", 1024), Path: "/workspace/SKILL.md"}
	}
	if got := Catalog(entries); len(got) > MaxCatalogBytes {
		t.Fatalf("rendered %d bytes", len(got))
	}
}
