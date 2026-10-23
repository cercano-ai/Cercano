//go:build windows

package filelock

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile reports whether the exclusive lock was taken without blocking.
func lockFile(f *os.File) (bool, error) {
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	switch err {
	case nil:
		return true, nil
	case windows.ERROR_LOCK_VIOLATION:
		return false, nil
	default:
		return false, err
	}
}

func unlockFile(f *os.File) {
	ol := new(windows.Overlapped)
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
