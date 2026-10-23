//go:build unix

package filelock

import (
	"os"

	"golang.org/x/sys/unix"
)

// lockFile reports whether the exclusive lock was taken without blocking.
func lockFile(f *os.File) (bool, error) {
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		switch err {
		case nil:
			return true, nil
		case unix.EINTR:
			continue
		case unix.EWOULDBLOCK:
			return false, nil
		default:
			return false, err
		}
	}
}

func unlockFile(f *os.File) {
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
