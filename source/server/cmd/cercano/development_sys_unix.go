//go:build !windows

package main

import (
	"os"
	"syscall"
)

// Unix implementation of the file and process primitives used by the
// development host launcher. Behaviour is unchanged from the inline calls
// these helpers replace.

const devOpenNoFollow = syscall.O_NOFOLLOW

const (
	devLockExclusive = syscall.LOCK_EX
	devLockUnlock    = syscall.LOCK_UN
	devLockNonBlock  = syscall.LOCK_NB
)

func devFlock(f *os.File, how int) error {
	return syscall.Flock(int(f.Fd()), how)
}

// devOwnedByCaller reports whether fi is owned by the calling user.
func devOwnedByCaller(fi os.FileInfo) (bool, error) {
	owner, ok := fi.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == uint32(os.Getuid()), nil
}

func devDetachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// devHostSupported gates management before config, store, or lock-file writes.
func devHostSupported() error { return nil }
