//go:build windows

package main

import (
	"bytes"
	"cercano/source/server/internal/conversation"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func windowsUpgradeFixture(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "PrivateUpgrade")
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := windows.UTF16PtrFromString(path)
	if err = windows.CreateDirectory(p, &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}); err != nil {
		t.Fatal(err)
	}
	return path
}
func windowsUpgradeArgs(root string) []string {
	return []string{"--database", filepath.Join(root, "conversations.db"), "--all-legacy-agents-stopped"}
}

// Production entrypoint: neither guard nor native discovery is injected.
func TestWindowsUpgradeProduction(t *testing.T) {
	root := windowsUpgradeFixture(t)
	legacy, err := conversation.Open(filepath.Join(root, "conversations.db"))
	if err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	db, err := sql.Open("sqlite", filepath.Join(root, "conversations.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TABLE fixture (value TEXT); INSERT INTO fixture VALUES ('preserved')")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err = runUpgradeStore(context.Background(), windowsUpgradeArgs(root), io.Discard); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", filepath.Join(root, "conversations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err = db.QueryRow("SELECT value FROM fixture").Scan(&value); err != nil || value != "preserved" {
		t.Fatalf("%q %v", value, err)
	}
}
func TestWindowsUpgradeLockCancellation(t *testing.T) {
	root := windowsUpgradeFixture(t)
	ctx := context.Background()
	release, err := developmentUpgradeGuard(ctx, filepath.Join(root, "conversations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	wait, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err = runUpgradeStore(wait, append(windowsUpgradeArgs(root), "--check-only"), io.Discard)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	release()
	if err = runUpgradeStore(ctx, append(windowsUpgradeArgs(root), "--check-only"), io.Discard); err != nil {
		t.Fatal(err)
	}
}
func TestWindowsUpgradeInventoryRefusal(t *testing.T) {
	for _, tc := range []struct{ name, content, want string }{
		{"malformed", "{", "JSON"},
		{"semantic", `{"host":"renamed"}`, "invalid start intent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := windowsUpgradeFixture(t)
			hosts := filepath.Join(root, "hosts")
			if err := os.Mkdir(hosts, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(hosts, "retained.starting")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			err := runUpgradeStore(context.Background(), windowsUpgradeArgs(root), io.Discard)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(path)
			if string(b) != tc.content {
				t.Fatal("inventory changed")
			}
		})
	}
	if err := upgradeRecordedHostAbsent(context.Background(), "renamed"); err == nil || !strings.Contains(err.Error(), "renamed") {
		t.Fatal(err)
	}
}
func TestWindowsUpgradeStopUnsupportedNoEffects(t *testing.T) {
	root := windowsUpgradeFixture(t)
	err := runUpgradeStore(context.Background(), append(windowsUpgradeArgs(root), "--stop-hosts-config", "missing"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal(entries)
	}
}
func TestWindowsUpgradeProcessIdentity(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if err = checkWindowsUpgradeProcess(uint32(os.Getpid()), user.User.Sid); err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatal(err)
	}
	if err = checkWindowsUpgradeProcess(0, user.User.Sid); err == nil {
		t.Fatal("unknown process accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(knownUpgradeAgents(ctx), context.Canceled) {
		t.Fatal("cancellation ignored")
	}
}

func TestWindowsUpgradeChild(t *testing.T) {
	if os.Getenv("CERCANO_UPGRADE_TEST_CHILD") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
}
func TestWindowsUpgradeNativeKnownProcessAndExit(t *testing.T) {
	root := windowsUpgradeFixture(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "cercano.exe")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path, "-test.run=^TestWindowsUpgradeChild$")
	cmd.Env = append(os.Environ(), "CERCANO_UPGRADE_TEST_CHILD=1")
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	err = runUpgradeStore(context.Background(), append(windowsUpgradeArgs(root), "--check-only"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatal(err)
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if err = checkWindowsUpgradeProcessHandle(h, uint32(cmd.Process.Pid), user.User.Sid); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsUpgradeUnsafeInventory(t *testing.T) {
	for _, kind := range []string{"directory", "hardlink", "unreadable"} {
		t.Run(kind, func(t *testing.T) {
			root := windowsUpgradeFixture(t)
			hosts := filepath.Join(root, "hosts")
			if err := os.Mkdir(hosts, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(hosts, "bad.json")
			var held windows.Handle
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				target := filepath.Join(root, "target")
				if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, path); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
				p, _ := windows.UTF16PtrFromString(path)
				var err error
				held, err = windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer windows.CloseHandle(held)
			}
			if err := runUpgradeStore(context.Background(), windowsUpgradeArgs(root), io.Discard); err == nil {
				t.Fatal("unsafe inventory accepted")
			}
			if _, err := os.Stat(filepath.Join(root, "conversations.db")); !os.IsNotExist(err) {
				t.Fatal(err)
			}
		})
	}
}

func TestWindowsUpgradeRetainedHost(t *testing.T) {
	root := windowsUpgradeFixture(t)
	hosts := filepath.Join(root, "hosts")
	if err := os.Mkdir(hosts, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hosts, "retained.json")
	data, err := json.Marshal(map[string]any{"TokenFile": filepath.Join(hosts, "retained.token"), "Identity": map[string]string{"host": "arbitrary-renamed-host", "store": filepath.Join(root, "conversations.db")}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	err = runUpgradeStore(context.Background(), windowsUpgradeArgs(root), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "renamed executables") {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(data) {
		t.Fatal("retained record changed")
	}
}

func TestWindowsUpgradeReparseInventory(t *testing.T) {
	root := windowsUpgradeFixture(t)
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	hosts := filepath.Join(root, "hosts")
	if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", hosts, target).CombinedOutput(); err != nil {
		t.Fatalf("disposable junction: %v %s", err, out)
	}
	defer os.Remove(hosts)
	if err := runUpgradeStore(context.Background(), windowsUpgradeArgs(root), io.Discard); err == nil || !strings.Contains(err.Error(), "unsupported coexistence object type") {
		t.Fatal(err)
	}
}

// Check the real Windows guard, including case-sensitive suffix bypasses, before
// native detection or any database mutation. All state is disposable fixture data.
func TestWindowsUpgradeInventoryEntryNames(t *testing.T) {
	for _, name := range []string{"retained.JSON", "retained.STARTING", "unknown-name", "retained.token", "unknown-directory", "directory.json"} {
		t.Run(name, func(t *testing.T) {
			root := windowsUpgradeFixture(t)
			database := filepath.Join(root, "conversations.db")
			db, err := conversation.Open(database)
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
			before, err := os.ReadFile(database)
			if err != nil {
				t.Fatal(err)
			}
			hosts := filepath.Join(root, "hosts")
			if err := os.Mkdir(hosts, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(hosts, name)
			data := []byte(`{"Identity":{"host":"retained"}}`)
			directory := name == "unknown-directory" || name == "directory.json"
			if directory {
				err = os.Mkdir(path, 0700)
			} else {
				err = os.WriteFile(path, data, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			// Production check-only reproduces acceptance without risking a migration.
			err = runUpgradeStore(context.Background(), append(windowsUpgradeArgs(root), "--check-only"), io.Discard)
			if err == nil {
				t.Errorf("inventory entry %q accepted by native Windows guard", name)
			}
			for _, checkOnly := range []bool{true, false} {
				detected := false
				args := windowsUpgradeArgs(root)
				if checkOnly {
					args = append(args, "--check-only")
				}
				err = runUpgradeStoreChecked(context.Background(), args, io.Discard, func(context.Context) error { detected = true; return nil })
				if err == nil || !strings.Contains(err.Error(), "development host inventory") || detected {
					t.Errorf("checkOnly=%v: refusal must precede detection: err=%v detected=%v", checkOnly, err, detected)
				}
			}
			after, err := os.ReadFile(database)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("fixture database changed: %v", err)
			}
			entries, err := os.ReadDir(hosts)
			if err != nil || len(entries) != 1 || entries[0].Name() != name || entries[0].IsDir() != directory {
				t.Fatalf("inventory changed: %v %v", entries, err)
			}
			if !directory {
				after, err = os.ReadFile(path)
				if err != nil || !bytes.Equal(data, after) {
					t.Fatalf("inventory content changed: %v", err)
				}
			}
			backups, err := filepath.Glob(database + ".pre-upgrade*")
			if err != nil || len(backups) != 0 {
				t.Fatalf("unexpected backup: %v %v", backups, err)
			}
		})
	}
}
