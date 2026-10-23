package filelock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func lockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "sub", "x.lock")
}

func TestTryLockExcludesSecondHolder(t *testing.T) {
	p := lockPath(t)
	release, ok, err := TryLock(p)
	if err != nil || !ok {
		t.Fatalf("first TryLock: ok=%v err=%v", ok, err)
	}
	defer release()
	release2, ok, err := TryLock(p)
	if err != nil {
		t.Fatalf("second TryLock err: %v", err)
	}
	if ok {
		release2()
		t.Fatal("second TryLock acquired a held lock")
	}
}

func TestReleaseAllowsRelock(t *testing.T) {
	p := lockPath(t)
	release, ok, err := TryLock(p)
	if err != nil || !ok {
		t.Fatalf("TryLock: ok=%v err=%v", ok, err)
	}
	release()
	release2, ok, err := TryLock(p)
	if err != nil || !ok {
		t.Fatalf("relock: ok=%v err=%v", ok, err)
	}
	release2()
}

func TestLockReturnsWhenContextDone(t *testing.T) {
	p := lockPath(t)
	release, ok, err := TryLock(p)
	if err != nil || !ok {
		t.Fatalf("TryLock: ok=%v err=%v", ok, err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	r, err := Lock(ctx, p)
	if r != nil {
		r()
		t.Fatal("Lock returned a release func for a held lock")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("Lock took %v", time.Since(start))
	}
}

func TestLockAcquiresAfterRelease(t *testing.T) {
	p := lockPath(t)
	release, _, _ := TryLock(p)
	go func() {
		time.Sleep(100 * time.Millisecond)
		release()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := Lock(ctx, p)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	r()
}

func TestTryLockCreatesPrivateFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions only")
	}
	p := lockPath(t)
	release, ok, err := TryLock(p)
	if err != nil || !ok {
		t.Fatalf("TryLock: ok=%v err=%v", ok, err)
	}
	defer release()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", fi.Mode().Perm())
	}
	di, err := os.Stat(filepath.Dir(p))
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, want 0700", di.Mode().Perm())
	}
}
