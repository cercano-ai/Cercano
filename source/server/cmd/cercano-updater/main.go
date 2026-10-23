// Command cercano-updater is the bundled, short-lived updater entrypoint.
// Execution is deliberately unavailable until the controlled handoff is wired.
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

var version = "dev"

func run(args []string, out, errOut io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(out, "cercano-updater v%s\n", version)
		return 0
	}
	// No permissive flag parsing, environment-selected mode, input reads, locks,
	// network or filesystem writes before a supported invocation is established.
	if len(args) != 1 || args[0] != "execute" {
		fmt.Fprintln(errOut, "usage: cercano-updater version | execute")
		return 2
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		fmt.Fprintln(errOut, "updater: unsupported platform")
		return 1
	}
	fmt.Fprintln(errOut, "updater: execution unwired; no handoff, drain or publication performed")
	return 1
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
