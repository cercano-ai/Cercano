package conversation

import (
	"context"
	"errors"
	"fmt"
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
	db, err := openCoexistenceDB(path, "ro")
	mustCoexist(t, err)
	defer db.Close()
	for i := 0; i < 2; i++ {
		var timeout int
		mustCoexist(t, db.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&timeout))
		if timeout != 5000 {
			t.Fatalf("read-only busy_timeout = %d, want 5000", timeout)
		}
		// Force the pool to replace its connection; the setting must survive.
		db.SetMaxIdleConns(0)
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
	if elapsed := time.Since(started); elapsed > 6*time.Second {
		t.Fatalf("locked preflight exceeded bounded wait: %s", elapsed)
	}
}
