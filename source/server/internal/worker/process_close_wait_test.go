//go:build unix

package worker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// Observe the ProcessState populated by the production Kill's cmd.Wait, not
// socket removal or signal-0 (which cannot distinguish a zombie).
func TestProcessCloseWaitResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/cat")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	h := &workerHandle{cmd: cmd}
	joined := make(chan struct{})
	go func() { defer close(joined); h.Kill() }()
	select {
	case <-joined:
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-joined
		t.Fatal("production Kill/Wait exceeded deadline")
	}
	_ = h.waitReaped(ctx) // Kill initiates retirement; only the owned reaper joins Wait.
	state := cmd.ProcessState
	if state == nil || state.Pid() != pid {
		t.Fatalf("missing exact child Wait state: %v, want pid %d", state, pid)
	}
	status, ok := state.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatalf("unexpected child exit: %v", state)
	}
	if _, err := cmd.Process.Wait(); !errors.Is(err, syscall.ECHILD) && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("child was not already reaped: %v", err)
	}
}
