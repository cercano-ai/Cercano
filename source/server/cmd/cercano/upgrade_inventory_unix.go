//go:build !windows

package main

import (
	"cercano/source/server/internal/brewrestart"
	"context"
	"os"
	"path/filepath"
)

func openUpgradeInventory(ctx context.Context, state string) (func(), func(string, any) error, func(string) ([]os.DirEntry, error), error) {
	if err := devHostSupported(); err != nil {
		return nil, nil, nil, err
	}
	lock, err := os.OpenFile(filepath.Join(state, "current-target.lock"), os.O_CREATE|os.O_RDWR|devOpenNoFollow, 0600)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = devFlock(lock, devLockExclusive); err != nil {
		lock.Close()
		return nil, nil, nil, err
	}
	return func() { devFlock(lock, devLockUnlock); lock.Close() }, launchRead, func(path string) ([]os.DirEntry, error) {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return nil, err
		}
		if err := launchPrivateDir(path); err != nil {
			return nil, err
		}
		return os.ReadDir(path)
	}, nil
}

var upgradeRecordedHostAbsent = brewrestart.VerifyHostAbsent

// Unix retains its existing ancillary-entry policy.
func validateUpgradeInventoryEntry(os.DirEntry) error { return nil }
