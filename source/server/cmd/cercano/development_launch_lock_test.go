//go:build !windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestLaunchLockContentionCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	owner, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	waiter, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer waiter.Close()
	if err := syscall.Flock(int(owner.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(owner.Fd()), syscall.LOCK_UN)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- launchLock(ctx, waiter) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cancellation: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		syscall.Flock(int(owner.Fd()), syscall.LOCK_UN)
		<-done
		syscall.Flock(int(waiter.Fd()), syscall.LOCK_UN)
		t.Fatal("contended launch lock ignored context deadline")
	}
	syscall.Flock(int(owner.Fd()), syscall.LOCK_UN)
	if err := launchLock(context.Background(), waiter); err != nil {
		t.Fatal(err)
	}
	syscall.Flock(int(waiter.Fd()), syscall.LOCK_UN)
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if err := launchLock(canceled, waiter); !errors.Is(err, context.Canceled) {
		t.Fatalf("already canceled: %v", err)
	}
}
