package runtimebundle

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T) (string, string, []Member) {
	t.Helper()
	source, retained := t.TempDir(), t.TempDir()
	var err error
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	retained, err = filepath.EvalSymlinks(retained)
	if err != nil {
		t.Fatal(err)
	}
	members := []Member{}
	for name, data := range map[string]string{"bin/agent": "binary", "assets/A-only": "asset", "manifest.json": `{"generation":"A"}`} {
		dest := filepath.Join(source, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		members = append(members, Member{Path: name, SHA256: sha256.Sum256([]byte(data)), Executable: name == "bin/agent"})
	}
	return source, retained, members
}

func TestPublishAndFailuresPreservePrior(t *testing.T) {
	for _, failure := range []string{"missing", "tampered", "cancelled", "interrupted", "symlink", "directory"} {
		t.Run(failure, func(t *testing.T) {
			source, retained, members := fixture(t)
			prior, err := Publish(context.Background(), source, retained, members)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			target := filepath.Join(source, "assets", "A-only")
			switch failure {
			case "missing":
				err = os.Remove(target)
			case "tampered":
				err = os.WriteFile(target, []byte("wrong"), 0600)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "interrupted":
				ctx = &interruptContext{Context: ctx, remaining: 3}
			case "symlink":
				if err = os.Remove(target); err == nil {
					err = os.Symlink(filepath.Join(source, "manifest.json"), target)
				}
			case "directory":
				if err = os.Remove(target); err == nil {
					err = os.Mkdir(target, 0700)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if got, err := Publish(ctx, source, retained, members); err == nil || got != "" {
				t.Fatalf("published invalid input: %q, %v", got, err)
			}
			if err := Validate(context.Background(), prior, members); err != nil {
				t.Fatalf("prior damaged: %v", err)
			}
			eligible, err := filepath.Glob(filepath.Join(retained, "generation-*", "bundle"))
			if err != nil || len(eligible) != 1 || eligible[0] != prior {
				t.Fatalf("eligibility: %v, %v", eligible, err)
			}
			if failure == "interrupted" {
				stages, _ := filepath.Glob(filepath.Join(retained, "generation-*", ".staging"))
				if len(stages) != 1 {
					t.Fatalf("expected abandoned ineligible stage: %v", stages)
				}
			}
		})
	}
}

// Deterministic cancellation during copy, without timing or production hooks.
type interruptContext struct {
	context.Context
	remaining int
}

func (c *interruptContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}

func TestInventoryAndSymlinkParents(t *testing.T) {
	source, retained, members := fixture(t)
	for _, name := range []string{"", ".", "..", "../escape", "/absolute", "a/../b", "a//b", "a\\b", "C:/absolute", "a\x00b"} {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			if got, err := Publish(context.Background(), source, retained, []Member{{Path: name}}); err == nil || got != "" {
				t.Fatalf("accepted %q", name)
			}
		})
	}
	for _, bad := range [][]Member{nil, {members[0], members[0]}, {{Path: "a"}, {Path: "a/b"}}} {
		if _, err := Publish(context.Background(), source, retained, bad); err == nil {
			t.Fatal("accepted invalid inventory")
		}
	}
	if err := os.Rename(filepath.Join(source, "assets"), filepath.Join(source, "real-assets")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real-assets", filepath.Join(source, "assets")); err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(context.Background(), source, retained, members); err == nil {
		t.Fatal("accepted symlink parent")
	}
}

func TestConcurrentIndependentPublications(t *testing.T) {
	source, retained, members := fixture(t)
	var wg sync.WaitGroup
	paths := make(chan string, 4)
	for range 4 {
		wg.Go(func() {
			got, err := Publish(context.Background(), source, retained, members)
			if err != nil {
				t.Error(err)
				return
			}
			paths <- got
		})
	}
	wg.Wait()
	close(paths)
	seen := map[string]bool{}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	for got := range paths {
		if seen[got] {
			t.Fatal("reused publication")
		}
		seen[got] = true
		if err := Validate(context.Background(), got, members); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 4 {
		t.Fatal("missing publication")
	}
}

func TestValidateDetectsRetainedTamper(t *testing.T) {
	source, retained, members := fixture(t)
	got, err := Publish(context.Background(), source, retained, members)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(got, "assets", "A-only"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(context.Background(), got, members); err == nil {
		t.Fatal("accepted retained tamper")
	}
}

func TestPublicationBoundaries(t *testing.T) {
	source, retained, members := fixture(t)
	nested := filepath.Join(source, "retained")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{source, source}, {source, nested}, {nested, source}} {
		if _, err := Publish(context.Background(), pair[0], pair[1], members); err == nil {
			t.Fatal("accepted overlapping roots")
		}
	}
	if err := os.WriteFile(filepath.Join(source, "unlisted"), []byte("not retained"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Publish(context.Background(), source, retained, members)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(got, "unlisted")); !os.IsNotExist(err) {
		t.Fatalf("copied unlisted member: %v", err)
	}
	if runtime.GOOS != "windows" {
		for _, name := range []string{got, filepath.Dir(got), filepath.Join(got, "bin", "agent")} {
			info, err := os.Stat(name)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0700 {
				t.Fatalf("not private/executable: %s %v", name, info.Mode())
			}
		}
		if err := os.Chmod(filepath.Join(got, "bin", "agent"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := Validate(context.Background(), got, members); err == nil {
			t.Fatal("accepted nonexecutable binary")
		}
	}
}
