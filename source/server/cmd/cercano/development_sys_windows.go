//go:build windows

package main

import (
	"errors"
	"os"
	"syscall"
)

// Windows stubs for the development host launcher. The launcher's safety
// checks (private, caller-owned state files, no-follow opens, flock) have no
// direct Windows equivalent yet, so development host management refuses to
// run here rather than weakening those checks. Everything else in the binary
// builds and runs as before.

var errDevHostUnsupported = errors.New("development host management is not supported on Windows yet")

const devOpenNoFollow = 0

const (
	devLockExclusive = 1 << iota
	devLockUnlock
	devLockNonBlock
)

func devFlock(*os.File, int) error { return errDevHostUnsupported }

func devOwnedByCaller(os.FileInfo) (bool, error) { return false, errDevHostUnsupported }

func devDetachSysProcAttr() *syscall.SysProcAttr { return nil }

// devHostSupported gates management before config, store, or lock-file writes.
func devHostSupported() error { return errDevHostUnsupported }
