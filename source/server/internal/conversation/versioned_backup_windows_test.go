package conversation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsBackupPublicationOrdering(t *testing.T) {
	for _, failure := range []string{"none", "flush", "publication"} {
		t.Run(failure, func(t *testing.T) {
			p := offlinePath(t)
			if err := PrepareCoexistence(context.Background(), p); err != nil {
				t.Fatal(err)
			}
			sentinel := errors.New("injected durability failure")
			var flushed *os.File
			var staging string
			published := false
			err := backupOfflineWindows(context.Background(), p, func(f *os.File) error {
				flushed = f
				staging = f.Name()
				if filepath.Dir(staging) != filepath.Dir(p) || staging == p+".pre-upgrade-backup" {
					t.Fatal("not same-directory staging", staging)
				}
				if _, err := os.Lstat(p + ".pre-upgrade-backup"); !os.IsNotExist(err) {
					t.Fatal("published before flush", err)
				}
				h, err := windowsPrivateHandle(staging, false)
				if err != nil {
					t.Fatal("snapshot not private", err)
				}
				windows.CloseHandle(h)
				if failure == "flush" {
					return sentinel
				}
				return f.Sync()
			}, func(from, to *uint16, flags uint32) error {
				published = true
				if flushed == nil {
					t.Fatal("publication before flush")
				}
				if _, err := flushed.Stat(); !errors.Is(err, windows.ERROR_INVALID_HANDLE) && !errors.Is(err, os.ErrClosed) {
					t.Fatal("publication before close", err)
				}
				if flags != windows.MOVEFILE_WRITE_THROUGH {
					t.Fatalf("unsafe publication flags: %#x", flags)
				}
				if windows.UTF16PtrToString(from) != staging || windows.UTF16PtrToString(to) != p+".pre-upgrade-backup" {
					t.Fatal("wrong publication paths")
				}
				if failure == "publication" {
					return sentinel
				}
				return windows.MoveFileEx(from, to, flags)
			})
			if failure == "none" {
				if err != nil || !published {
					t.Fatal(err)
				}
				if _, err := os.Lstat(staging); !os.IsNotExist(err) {
					t.Fatal("staging not moved", err)
				}
				if err := PreflightCoexistence(context.Background(), p+".pre-upgrade-backup", CurrentCoexistenceDeclaration()); err != nil {
					t.Fatal(err)
				}
			} else {
				if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), fmt.Sprintf("%q", staging)) {
					t.Fatal("missing recovery diagnostic", err)
				}
				if published != (failure == "publication") {
					t.Fatal("wrong publication order")
				}
				if err := PreflightCoexistence(context.Background(), staging, CurrentCoexistenceDeclaration()); err != nil {
					t.Fatal("recovery snapshot unusable", err)
				}
				if _, err := os.Lstat(p + ".pre-upgrade-backup"); !os.IsNotExist(err) {
					t.Fatal("failure published final name", err)
				}
			}
		})
	}
}

func TestWindowsBackupDestinationRaceBeforeMutation(t *testing.T) {
	ctx := context.Background()
	p := offlinePath(t)
	if err := prepareCoexistence(ctx, p, developmentMigrations[0].from); err != nil {
		t.Fatal(err)
	}
	db, err := openCoexistenceDB(p, "rw")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA wal_autocheckpoint=0", `INSERT INTO conversations(id,started_at,last_turn_at,project_dir) VALUES('live-wal',1,2,'/fixture')`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	mainBefore, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	walBefore, err := os.ReadFile(p + "-wal")
	if err != nil || len(walBefore) <= 32 {
		t.Fatal("missing live WAL", err)
	}
	if _, err := db.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("ROLLBACK")
	var staging string
	err = upgradeDeclaredWithSync(ctx, db, p, DevelopmentCoexistenceDeclaration(), developmentMigrations, func(f *os.File) error {
		staging = f.Name()
		// Compete after the initial destination absence check, before MoveFileEx.
		if err := os.WriteFile(p+".pre-upgrade-backup", []byte("racing destination must survive"), 0600); err != nil {
			return err
		}
		return f.Sync()
	})
	if err == nil || !strings.Contains(err.Error(), "publish offline upgrade backup") || !strings.Contains(err.Error(), fmt.Sprintf("%q", staging)) {
		t.Fatal("expected publication refusal with recovery path", err)
	}
	b, err := os.ReadFile(p + ".pre-upgrade-backup")
	if err != nil || string(b) != "racing destination must survive" {
		t.Fatal("destination overwritten", err)
	}
	recovery, err := openCoexistenceDB(staging, "ro")
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Close()
	var recovered string
	if err := recovery.QueryRow("PRAGMA integrity_check").Scan(&recovered); err != nil || recovered != "ok" {
		t.Fatal("lost recovery snapshot", recovered, err)
	}
	if err := recovery.QueryRow("SELECT id FROM conversations WHERE id='live-wal'").Scan(&recovered); err != nil || recovered != "live-wal" {
		t.Fatal("recovery lost committed WAL", recovered, err)
	}
	mainAfter, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(mainBefore, mainAfter) {
		t.Fatal("source main file changed", err)
	}
	walAfter, err := os.ReadFile(p + "-wal")
	if err != nil || !bytes.Equal(walBefore, walAfter) {
		t.Fatal("live WAL changed", err)
	}
	// Observe before rollback, which could otherwise conceal mutation.
	if exists, err := tableExists(db, "execution_bindings"); err != nil || exists {
		t.Fatal("source mutated", exists, err)
	}
	var contract string
	var version, uv int
	if err := db.QueryRow("SELECT contract,schema_version FROM coexistence_metadata").Scan(&contract, &version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if contract != developmentMigrations[0].from.Contract || version != 2 || uv != 2 {
		t.Fatal("source markers mutated", contract, version, uv)
	}
}
