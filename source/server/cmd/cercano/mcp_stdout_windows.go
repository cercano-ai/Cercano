//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// reserveStdoutForMCP keeps the original standard output for the MCP
// protocol and makes standard error the process's standard output: os.Stdout
// (which fmt.Print writes to and children are usually given) and the
// process's standard output handle (which native code and children that
// inherit standard handles use).
func reserveStdoutForMCP() (*os.File, error) {
	protocol := os.Stdout
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(os.Stderr.Fd())); err != nil {
		return nil, err
	}
	os.Stdout = os.Stderr
	return protocol, nil
}
