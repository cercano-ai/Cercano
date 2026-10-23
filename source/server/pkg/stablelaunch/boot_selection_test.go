package stablelaunch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cercano/source/server/internal/updatecoord/activation"
	"cercano/source/server/internal/updatecoord/exclusion"
	"cercano/source/server/internal/updatecoord/privdir"
	"cercano/source/server/internal/updatecoord/selection"
	"cercano/source/server/pkg/installcontext"
)

func TestBootRejectsWorldWritableSelection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix mode policy; Windows uses ACLs")
	}
	f := fixture(t)
	dir, err := installcontext.BootDirectory(f.StateRoot, f.InstallID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(filepath.Join(dir, selection.FileName), 0666); err != nil {
		t.Fatal(err)
	}
	t.Run("stablelaunch preparation", func(t *testing.T) {
		p, err := prepare(context.Background(), filepath.Join(f.Root, f.AgentName), f.StateRoot, nil, false)
		if p != nil {
			p.lock.Close()
		}
		if !errors.Is(err, activation.ErrSelectionUnreadable) {
			t.Fatalf("unsafe 0666 selection accepted or wrong refusal: %v", err)
		}
	})
	t.Run("VerifyChild", func(t *testing.T) {
		_, err := installcontext.VerifyChild(context.Background(), f.Locators(), f.StateRoot, f.AgentExe, false)
		if !errors.Is(err, activation.ErrSelectionUnreadable) {
			t.Fatalf("unsafe 0666 selection accepted or wrong refusal: %v", err)
		}
	})
}

// Exercise actual publisher output, including inherited private Windows ACLs.
func TestBootSelectionPublisherAndRefusals(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	dir, err := installcontext.BootDirectory(f.StateRoot, f.InstallID)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, selection.FileName)
	selected, err := activation.ReadSelection(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	lock, err := exclusion.Acquire(ctx, dir, exclusion.Update)
	if err != nil {
		t.Fatal(err)
	}
	result, err := selection.Publish(ctx, dir, lock, selection.Expected{Absent: true}, selected)
	lock.Close()
	if err != nil {
		t.Fatalf("publish: %+v %v", result, err)
	}
	check := func(t *testing.T, want error) {
		t.Helper()
		p, err := prepare(ctx, filepath.Join(f.Root, f.AgentName), f.StateRoot, nil, false)
		if p != nil {
			p.lock.Close()
		}
		if !errors.Is(err, want) {
			t.Fatalf("prepare: %v want %v", err, want)
		}
		_, err = installcontext.VerifyChild(ctx, f.Locators(), f.StateRoot, f.AgentExe, false)
		if !errors.Is(err, want) {
			t.Fatalf("child: %v want %v", err, want)
		}
	}
	check(t, nil)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want error
	}{
		{"corrupt", []byte("{"), activation.ErrSelectionMalformed},
		{"oversize", []byte(strings.Repeat(" ", 4097)), activation.ErrSelectionMalformed},
		{"missing", nil, activation.ErrSelectionAbsent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.data == nil {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, tc.data, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			check(t, tc.want)
		})
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return
	} // Unix modes below, not Windows ACL simulation.
	t.Run("nonprivate mode", func(t *testing.T) {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		check(t, activation.ErrSelectionUnreadable)
		if err := os.Chmod(path, 0600); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		backing := filepath.Join(dir, "selection-backing")
		if err := os.Rename(path, backing); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(backing, path); err != nil {
			t.Fatal(err)
		}
		check(t, activation.ErrSelectionUnreadable)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(backing, path); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		link := filepath.Join(dir, "selection-alias")
		if err := os.Link(path, link); err != nil {
			t.Fatal(err)
		}
		check(t, activation.ErrSelectionUnreadable)
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("nested staged ancestor", func(t *testing.T) {
		versions := filepath.Join(f.Root, "versions")
		nested := filepath.Join(versions, "nested")
		if _, err := privdir.Ensure(nested); err != nil {
			t.Fatal(err)
		}
		old := filepath.Join(versions, filepath.FromSlash(selected.StagedVersionDir))
		if err := os.Rename(old, filepath.Join(nested, "release")); err != nil {
			t.Fatal(err)
		}
		selected.StagedVersionDir = "nested/release"
		data, err := json.Marshal(selected)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		f.AgentExe = filepath.Join(nested, "release", "bin", f.AgentName)
		check(t, nil)
		if err = os.Chmod(nested, 0777); err != nil {
			t.Fatal(err)
		}
		check(t, privdir.ErrUnsafeACL)
	})
}
