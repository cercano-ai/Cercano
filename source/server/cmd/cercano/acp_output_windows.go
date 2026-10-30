//go:build windows

package main

import (
	"os"
	"syscall"
)

// Duplicate stdout so transport shutdown never closes the caller's handle.
// NewFile uses the runtime's best-effort cancellation of Windows pipe IO.
func acpOutput(stdout *os.File) (*os.File, error) {
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, err
	}
	var handle syscall.Handle
	if err := syscall.DuplicateHandle(process, syscall.Handle(stdout.Fd()), process, &handle, 0, false, syscall.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), "acp-output"), nil
}
