//go:build windows

package profilemigrate

import (
	"errors"
	"syscall"
)

// errCrossDevice is ERROR_NOT_SAME_DEVICE, which MoveFileEx reports when
// source and destination are on different volumes.
var errCrossDevice error = syscall.Errno(17)

func isCrossDevice(err error) bool { return errors.Is(err, errCrossDevice) }
