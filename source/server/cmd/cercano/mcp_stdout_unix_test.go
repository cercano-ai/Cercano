//go:build unix

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// mcpStdoutChildEnv runs TestReserveStdoutForMCP's child half.
const mcpStdoutChildEnv = "CERCANO_TEST_MCP_STDOUT_CHILD"

// Once standard output is reserved for MCP, only what is written to the
// reserved file reaches the real standard output: fmt.Print diagnostics,
// direct os.Stdout writes and a child process inheriting standard output all
// land on standard error, and the protocol file is close-on-exec, so no
// child inherits it.
func TestReserveStdoutForMCP(t *testing.T) {
	if os.Getenv(mcpStdoutChildEnv) == "1" {
		out, err := reserveStdoutForMCP()
		if err != nil {
			fmt.Fprintln(os.Stderr, "reserve:", err)
			os.Exit(3)
		}
		fmt.Printf("UpdateConfig: diagnostic via fmt.Printf\n")
		os.Stdout.WriteString("diagnostic via os.Stdout\n")
		if flags, err := unix.FcntlInt(out.Fd(), unix.F_GETFD, 0); err != nil || flags&unix.FD_CLOEXEC == 0 {
			fmt.Fprintln(os.Stderr, "the protocol file would be inherited by children:", flags, err)
			os.Exit(5)
		}
		child := exec.Command("/bin/sh", "-c", "echo diagnostic from a child")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "child:", err)
			os.Exit(4)
		}
		out.WriteString(`{"jsonrpc":"2.0","id":1,"result":{}}` + "\n")
		out.Close()
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestReserveStdoutForMCP$")
	cmd.Env = append(os.Environ(), mcpStdoutChildEnv+"=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("child: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got, want := stdout.String(), `{"jsonrpc":"2.0","id":1,"result":{}}`+"\n"; got != want {
		t.Fatalf("standard output = %q, want only the protocol message %q", got, want)
	}
	for _, want := range []string{"UpdateConfig: diagnostic via fmt.Printf", "diagnostic via os.Stdout", "diagnostic from a child"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("standard error lacks %q:\n%s", want, stderr.String())
		}
	}
}
