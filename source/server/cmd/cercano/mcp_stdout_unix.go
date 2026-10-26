//go:build unix

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// reserveStdoutForMCP keeps a close-on-exec duplicate of standard output for
// the MCP protocol and points file descriptor 1 at standard error, so any
// other write to standard output — from this process or a child that
// inherits it — lands on standard error.
func reserveStdoutForMCP() (*os.File, error) {
	fd, err := unix.FcntlInt(uintptr(unix.Stdout), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	if err := unix.Dup2(unix.Stderr, unix.Stdout); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), "mcp-protocol-stdout"), nil
}
