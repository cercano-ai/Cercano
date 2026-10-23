// Command cercano-updater is the bundled, short-lived updater entrypoint.
// Execute connects only owner/identity/drain evidence; installation completion
// remains deliberately unavailable.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
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
	if len(args) != 3 || args[0] != "execute" || args[1] != "--input" {
		fmt.Fprintln(errOut, "usage: cercano-updater version | execute --input /absolute/operation.json")
		return 2
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		fmt.Fprintln(errOut, "updater: unsupported platform")
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	err := execute(ctx, args[2])
	if err == nil {
		err = errIncomplete
	}
	fmt.Fprintln(errOut, err)
	return 1
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
