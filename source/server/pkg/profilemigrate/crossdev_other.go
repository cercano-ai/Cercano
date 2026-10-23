//go:build !windows

package profilemigrate

import (
	"errors"
	"syscall"
)

// errCrossDevice is the error rename reports when source and destination are
// on different filesystems.
var errCrossDevice error = syscall.EXDEV

func isCrossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }

func isConnRefused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }
