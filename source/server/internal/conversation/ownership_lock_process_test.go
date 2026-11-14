//go:build unix || windows

package conversation

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestOwnershipLockProcessExit(t *testing.T) {
	if path := os.Getenv("CERCANO_LOCK_PROCESS_CHILD"); path != "" {
		f, err := os.OpenFile(path, os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err = lockOwnershipFile(f); err != nil {
			t.Fatal(err)
		}
		fmt.Println("locked")
		var b [1]byte
		os.Stdin.Read(b[:])
		// Deliberately no Close: verify kernel cleanup, not a deferred unlock.
		os.Exit(0)
	}
	f, err := os.CreateTemp(t.TempDir(), "lifetime-lock")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOwnershipLockProcessExit$")
	cmd.Env = append(os.Environ(), "CERCANO_LOCK_PROCESS_CHILD="+f.Name())
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		in.Close()
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("child readiness %q: %v", line, err)
	}
	if err = lockOwnershipFile(f); !ownershipLockContended(err) {
		t.Fatalf("expected classified contention: %v", err)
	}
	if err = in.Close(); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err = lockOwnershipFile(f); err != nil {
		t.Fatalf("exit did not release lock: %v", err)
	}
}
