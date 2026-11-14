package conversation

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Exercise the launcher's read-only admission alongside fresh ownership writes.
func TestCoexistenceConcurrentPreflightAcquire(t *testing.T) {
	ctx, path, store := developmentOwnershipFixture(t)
	for i := 0; i < 64; i++ {
		start := make(chan struct{})
		read := make(chan error, 1)
		go func() {
			<-start
			read <- PreflightCoexistence(ctx, path, DevelopmentCoexistenceDeclaration())
		}()
		close(start)
		owner, err := AcquireOwnership(ctx, store, fmt.Sprintf("probe-%d", i), "same-generation")
		mustCoexist(t, err)
		mustCoexist(t, <-read)
		mustCoexist(t, owner.Release(ctx))
	}
}

// Rollback-journal exclusive writes give a deterministic transient read lock,
// including on machines where the concurrent WAL recovery race is rare.
func TestCoexistenceReadOnlyWaitsForTransientLock(t *testing.T) {
	ctx, path, store := developmentOwnershipFixture(t)
	_, err := store.db.Exec("PRAGMA journal_mode=DELETE; BEGIN EXCLUSIVE;")
	mustCoexist(t, err)
	defer store.db.Exec("ROLLBACK")
	done := make(chan error, 1)
	go func() { done <- PreflightCoexistence(ctx, path, DevelopmentCoexistenceDeclaration()) }()
	select {
	case err := <-done:
		t.Fatalf("preflight returned before transient writer released: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	_, err = store.db.Exec("COMMIT")
	mustCoexist(t, err)
	select {
	case err := <-done:
		mustCoexist(t, err)
	case <-time.After(6 * time.Second):
		t.Fatal("preflight did not resume after writer released")
	}
}

func TestCoexistenceReadOnlyConnectionTimeout(t *testing.T) {
	_, path, _ := developmentOwnershipFixture(t)
	for _, tc := range []struct {
		name string
		open func(string) (*sql.DB, error)
		want int
	}{
		{"generic", func(path string) (*sql.DB, error) { return openCoexistenceDB(path, "ro") }, 5000},
		{"preflight", openPreflightCoexistenceDB, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := tc.open(path)
			mustCoexist(t, err)
			defer db.Close()
			for i := 0; i < 3; i++ {
				conn, err := db.Conn(context.Background())
				mustCoexist(t, err)
				var timeout int
				err = conn.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&timeout)
				mustCoexist(t, err)
				if timeout != tc.want {
					t.Fatalf("read-only busy_timeout = %d, want %d", timeout, tc.want)
				}
				// Read-only mode must survive replacement along with busy_timeout.
				_, err = conn.ExecContext(context.Background(), "CREATE TABLE forbidden (id INTEGER)")
				var coded interface{ Code() int }
				if !errors.As(err, &coded) || coded.Code()&0xff != 8 { // SQLITE_READONLY
					t.Fatalf("read-only write: %v", err)
				}
				// Returning this connection closes it, forcing a fresh driver connection.
				db.SetMaxIdleConns(0)
				mustCoexist(t, conn.Close())
			}
		})
	}
}

func TestCoexistenceReadOnlyLockDeadline(t *testing.T) {
	ctx, path, store := developmentOwnershipFixture(t)
	_, err := store.db.Exec("PRAGMA journal_mode=DELETE; BEGIN EXCLUSIVE;")
	mustCoexist(t, err)
	defer store.db.Exec("ROLLBACK")
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = PreflightCoexistence(ctx, path, DevelopmentCoexistenceDeclaration())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("locked preflight: got %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("locked preflight exceeded bounded wait: %s", elapsed)
	}
}

func TestCoexistenceReadOnlyLockCanceled(t *testing.T) {
	ctx, path, store := developmentOwnershipFixture(t)
	_, err := store.db.Exec("PRAGMA journal_mode=DELETE; BEGIN EXCLUSIVE;")
	mustCoexist(t, err)
	defer store.db.Exec("ROLLBACK")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	started := time.Now()
	err = PreflightCoexistence(ctx, path, DevelopmentCoexistenceDeclaration())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("locked preflight: got %v, want canceled", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("locked preflight exceeded bounded wait: %s", elapsed)
	}
}

func TestCoexistencePreflightNoMutation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fixture %#.db")
	mustCoexist(t, PrepareCoexistence(ctx, path))
	// Use a closed rollback-journal fixture to compare durable bytes and ensure
	// admission creates no sidecars, schema, or other files.
	db, err := openCoexistenceDB(path, "rw")
	mustCoexist(t, err)
	_, err = db.Exec("PRAGMA journal_mode=DELETE")
	mustCoexist(t, err)
	mustCoexist(t, db.Close())
	before, err := os.ReadFile(path)
	mustCoexist(t, err)
	mustCoexist(t, PreflightCoexistence(ctx, path, CurrentCoexistenceDeclaration()))
	if err := PreflightCoexistence(ctx, path, DevelopmentCoexistenceDeclaration()); err == nil {
		t.Fatal("accepted wrong declaration")
	}
	after, err := os.ReadFile(path)
	mustCoexist(t, err)
	if !bytes.Equal(before, after) {
		t.Fatal("preflight changed database bytes")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	mustCoexist(t, err)
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("preflight created files: %v", entries)
	}
}
