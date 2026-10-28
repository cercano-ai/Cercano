package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func boundaryDraft(t *testing.T, p string) Config {
	t.Helper()
	c, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func boundarySave(t *testing.T, c Config, p string) Config {
	t.Helper()
	next, e := SaveSnapshot(c, p, c.Snapshot())
	if e != nil {
		t.Fatal(e)
	}
	return c.WithSnapshot(next)
}
func TestSaveBoundaryStaleDraft(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("port: '1234'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := boundaryDraft(t, p)
	b := a.Clone()
	a.Port = "2345"
	a = boundarySave(t, a, p)
	a.Port = "4567"
	boundarySave(t, a, p)
	b.Port = "3456"
	if err := Save(b, p); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("stale draft: %v", err)
	}
}
func TestSaveBoundaryCreationAndCompetingWriters(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "creation", true: "existing"}[existing], func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "nested", "config.yaml")
			a := boundaryDraft(t, p)
			if existing {
				a = boundarySave(t, a, p)
			}
			b := boundaryDraft(t, p)
			var wg sync.WaitGroup
			results := make(chan error, 2)
			for i, c := range []Config{a, b} {
				wg.Add(1)
				go func(i int, c Config) {
					defer wg.Done()
					c.Port = []string{"1234", "2345"}[i]
					_, e := SaveSnapshot(c, p, c.Snapshot())
					results <- e
				}(i, c)
			}
			wg.Wait()
			close(results)
			wins, conflicts := 0, 0
			for e := range results {
				if e == nil {
					wins++
				} else if errors.Is(e, ErrConfigConflict) {
					conflicts++
				} else {
					t.Fatal(e)
				}
			}
			if wins != 1 || conflicts != 1 {
				t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
			}
		})
	}
}
func TestSaveBoundaryFailClosed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	c := boundaryDraft(t, p)
	if _, e := SaveSnapshot(c, p, ConfigSnapshot{}); !errors.Is(e, ErrConfigConflict) {
		t.Fatalf("missing: %v", e)
	}
	other := filepath.Join(t.TempDir(), "config.yaml")
	if _, e := SaveSnapshot(c, other, c.Snapshot()); !errors.Is(e, ErrConfigConflict) {
		t.Fatalf("retarget: %v", e)
	}
	c = boundarySave(t, c, p)
	if e := os.Remove(p); e != nil {
		t.Fatal(e)
	}
	if _, e := SaveSnapshot(c, p, c.Snapshot()); !errors.Is(e, ErrConfigConflict) {
		t.Fatalf("disappearance: %v", e)
	}
}
func TestSaveBoundaryFailedPublication(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	c := boundarySave(t, boundaryDraft(t, p), p)
	before, _ := os.ReadFile(p)
	coordinator, e := NewConfigCoordinator(p)
	if e != nil {
		t.Fatal(e)
	}
	coordinator.publish = func(string, []byte, os.FileMode) error { return errors.New("fixture publication failure") }
	c.Port = "2345"
	next, e := coordinator.Save(context.Background(), c.Snapshot(), c)
	if e == nil || next.authority != "" {
		t.Fatal("publication failure returned success/baseline")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("failed save changed disk")
	}
}
func TestSaveBoundaryInvalidExisting(t *testing.T) {
	for _, kind := range []string{"malformed", "unreadable", "dangling"} {
		t.Run(kind, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.yaml")
			switch kind {
			case "malformed":
				os.WriteFile(p, []byte("secret-fixture: [unterminated"), 0600)
			case "unreadable":
				os.Mkdir(p, 0700)
			case "dangling":
				os.Symlink(filepath.Join(t.TempDir(), "absent"), p)
			}
			if _, e := Load(p); e == nil {
				t.Fatal("invalid existing authority admitted")
			}
			if _, e := SaveSnapshot(Defaults(), p, ConfigSnapshot{}); e == nil {
				t.Fatal("defaults overwrote existing authority")
			}
		})
	}
}

// saveFixture admits only a genuinely absent fixture when a serialization test
// starts from Defaults. Existing loaded drafts retain their original baseline.
func saveFixture(t *testing.T, c Config, p string) error {
	t.Helper()
	if c.Snapshot().authority == "" {
		base := boundaryDraft(t, p).Snapshot()
		if !base.absent {
			t.Fatal("fixture must explicitly load existing configuration")
		}
		c = c.WithSnapshot(base)
	}
	return Save(c, p)
}

func TestSaveBoundaryChangeDisappearance(t *testing.T) {
 p := filepath.Join(t.TempDir(), "config.yaml")
 c := boundarySave(t, boundaryDraft(t, p), p)
 coordinator, err := NewConfigCoordinator(p)
 if err != nil { t.Fatal(err) }
 if err := os.Remove(p); err != nil { t.Fatal(err) }
 _, err = coordinator.Change(context.Background(), c.Snapshot(), ConfigChange{Path: []string{"fixture_unknown"}, YAML: []byte("true")})
 if !errors.Is(err, ErrConfigConflict) { t.Fatalf("disappeared authority: %v", err) }
 if _, err := os.Stat(p); !os.IsNotExist(err) { t.Fatalf("authority recreated: %v", err) }
}
