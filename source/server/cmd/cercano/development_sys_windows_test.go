//go:build windows

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// These tests must run on a Windows host; cross-compilation is not acceptance.
func TestWindowsDevelopmentRefusalBeforeSideEffects(t *testing.T) {
	prepareConfigFixture(t)
	for _, name := range []string{"prepare", "launch", "receipt", "host", "upgrade", "instance"} {
		t.Run(name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(root, "sentinel")
			if err := os.WriteFile(sentinel, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			switch name {
			case "prepare":
				err = runPrepareDevelopment(ctx, []string{"--new-state", filepath.Join(root, "new")}, io.Discard)
			case "launch":
				err = runDevelopmentLaunch(ctx, []string{"--state", root, "--runtime-root", root}, io.Discard)
			case "receipt":
				err = runDevelopmentLaunchReceipt(ctx, []string{"--operation-file", sentinel, "--action", "abort"}, io.Discard)
			case "host":
				err = runDevelopmentHostWithFactory(ctx, nil, io.Discard, nil)
			case "upgrade":
				var release func()
				release, err = developmentUpgradeGuardWith(ctx, filepath.Join(root, "conversations.db"), func(context.Context, string) error { t.Fatal("process probe before refusal"); return nil })
				if release != nil {
					release()
					t.Fatal("returned release on unsupported platform")
				}
			case "instance":
				var f *os.File
				f, err = developmentInstanceLock(filepath.Join(root, "instance"))
				if f != nil {
					f.Close()
					t.Fatal("returned lock on unsupported platform")
				}
			}
			if !errors.Is(err, errDevHostUnsupported) {
				t.Errorf("want unsupported, got %v", err)
			}
			entries, readErr := os.ReadDir(root)
			if readErr != nil || len(entries) != 1 || entries[0].Name() != "sentinel" {
				t.Errorf("refusal changed state: %v, %v", entries, readErr)
			}
			b, err := os.ReadFile(sentinel)
			if err != nil || string(b) != "unchanged" {
				t.Fatalf("sentinel changed: %q, %v", b, err)
			}
		})
	}
}

func TestWindowsDevelopmentPrimitivesRefuse(t *testing.T) {
	if owned, err := devOwnedByCaller(nil); owned || !errors.Is(err, errDevHostUnsupported) {
		t.Fatalf("ownership: %v %v", owned, err)
	}
	if err := devFlock(nil, devLockExclusive|devLockNonBlock); !errors.Is(err, errDevHostUnsupported) {
		t.Fatal(err)
	}
	if err := launchLock(context.Background(), nil); !errors.Is(err, errDevHostUnsupported) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := launchLock(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
