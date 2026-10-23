// Package filelock provides a cross-platform exclusive advisory file lock.
package filelock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const pollInterval = 50 * time.Millisecond

// TryLock takes a non-blocking exclusive lock on path, creating the file
// (0600) and its parent directory (0700) if needed. ok is false with a nil
// error when another holder has the lock. The release function unlocks and
// closes the file; it never deletes it and is safe to call more than once.
func TryLock(path string) (release func(), ok bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, fmt.Errorf("create lock directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open lock file: %w", err)
	}
	locked, err := lockFile(f)
	if err != nil {
		_ = f.Close()
		return nil, false, fmt.Errorf("lock %s: %w", path, err)
	}
	if !locked {
		_ = f.Close()
		return nil, false, nil
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			unlockFile(f)
			_ = f.Close()
		})
	}, true, nil
}

// Lock blocks until the lock on path is acquired or ctx is done, polling
// TryLock every 50ms.
func Lock(ctx context.Context, path string) (release func(), err error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		release, ok, err := TryLock(path)
		if err != nil {
			return nil, err
		}
		if ok {
			return release, nil
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
