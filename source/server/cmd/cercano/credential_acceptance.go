package main

import (
	"fmt"
	"io"
)

const credentialAcceptanceCommand = "acceptance-read-existing-credential"

// This branch is deliberately before every ordinary startup path. The native
// implementation exists only in explicitly tagged Darwin acceptance builds.
func dispatchCredentialAcceptance(args []string, out io.Writer, query func() int) bool {
	if len(args) == 0 || args[0] != credentialAcceptanceCommand {
		return false
	}
	status := -50 // invalid invocation; do not query anything
	if len(args) == 1 {
		status = query()
	}
	// Only a numeric native status: 0 success, -25300 missing, other error.
	fmt.Fprintln(out, status)
	return true
}
