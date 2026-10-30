//go:build !windows

package main

import (
	"os"
	"syscall"
)

// acpOutput gives the transport an independently closable, pollable descriptor.
// Inherited stdout may use blocking writes that os.File.Close cannot interrupt.
// Set nonblocking before NewFile so Go registers pipes with its runtime poller.
func acpOutput(stdout *os.File) (*os.File, error) {
	fd, err := syscall.Dup(int(stdout.Fd()))
	if err != nil {
		return nil, err
	}
	syscall.CloseOnExec(fd)
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), "acp-output"), nil
}
