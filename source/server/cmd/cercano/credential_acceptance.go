package main

import (
	"fmt"
	"io"

	"cercano/source/server/internal/secrets"
)

const credentialAcceptanceCommand = "acceptance-read-existing-credential"

// This branch is deliberately before every ordinary startup path. The native
// implementation exists only in explicitly tagged Darwin acceptance builds.
func dispatchCredentialAcceptance(args []string, out io.Writer, query func(bool) int) bool {
	if len(args) == 0 || args[0] != credentialAcceptanceCommand {
		return false
	}
	status := -50 // invalid invocation; do not query anything
	if len(args) == 1 || (len(args) == 2 && args[1] == "--interactive") {
		status = query(len(args) == 2)
	}
	// Only a numeric native status: 0 success, -25300 missing, other error.
	fmt.Fprintln(out, status)
	return true
}

// Check policy before the injected native boundary, including malformed settings.
func runAcceptanceCredential(interactive bool, native func(bool) int) int {
	if secrets.CheckAllowed() != nil {
		return -4
	}
	return native(interactive)
}
