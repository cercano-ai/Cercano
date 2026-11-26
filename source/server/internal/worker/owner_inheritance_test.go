package worker

import (
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

// An unrelated host child stays alive across closing the host writer. If Go's
// spawn inherited that writer, EOF would not arrive until this child exited.
func TestOwnerPipeUnrelatedChild(t *testing.T) {
	if os.Getenv("CERCANO_OWNER_BYSTANDER") == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnerPipeUnrelatedChild$")
	cmd.Env = append(os.Environ(), "CERCANO_OWNER_BYSTANDER=1")
	control, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	r, w, err := prepareOwnerPipe(exec.Command(os.Args[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { control.Close(); cmd.Process.Kill(); cmd.Wait() }()
	w.Close()
	done := make(chan error, 1)
	go func() { var b [1]byte; _, err := r.Read(b[:]); done <- err }()
	select {
	case err := <-done:
		if err != io.EOF {
			t.Fatalf("read: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("owner writer leaked to unrelated child")
	}
}
