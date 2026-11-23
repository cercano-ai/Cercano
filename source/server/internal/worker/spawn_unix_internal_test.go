//go:build unix

package worker

// spawn_unix_internal_test.go — regression guard for the warm-pool lifetime bug.
//
// The pool keeps a conversation's worker WARM across turns. spawnWorker must
// therefore NOT bind the child process's lifetime to the turn ctx it was
// spawned under — otherwise the pooled worker is SIGKILLed the instant the
// first turn completes (its ctx cancels), and every turn silently respawns,
// defeating the entire warm-reuse feature. This test spawns a real worker,
// cancels the spawn ctx (simulating turn-end), and asserts the process lives.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSpawnWorker_ProcessOutlivesTurnContext(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the cercano binary; skipped under -short")
	}

	// Build the real cercano binary and make findWorkerBinary resolve it via PATH.
	dir := t.TempDir()
	bin := filepath.Join(dir, "cercano")
	build := exec.Command("go", "build", "-o", bin, "cercano/source/server/cmd/cercano")
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build cercano binary (skipping): %v\n%s", err, out)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(context.Background())
	h, err := spawnWorker(ctx, "outlives-turn", 1)
	if err != nil {
		cancel()
		t.Fatalf("spawnWorker: %v", err)
	}
	defer h.Kill()

	// Simulate the turn ending: cancel the ctx passed to spawnWorker. The pooled
	// worker MUST survive — its lifetime is owned by the pool (Kill/Shutdown/
	// reap/orphan-sweep), not the turn. Regression: exec.CommandContext(ctx,…)
	// SIGKILLs the process right here.
	cancel()
	time.Sleep(300 * time.Millisecond) // give any ctx-watcher time to (wrongly) act.

	// Probe liveness with Wait4(WNOHANG), NOT signal-0: exec.CommandContext would
	// SIGKILL the child on cancel, leaving a ZOMBIE, and signal-0 succeeds on a
	// zombie (so processAlive can't tell dead from alive here). Wait4 reaps and
	// reports the exit for a killed child, and returns 0 for a running one — so it
	// actually distinguishes the two.
	pid := h.cmd.Process.Pid
	var ws syscall.WaitStatus
	wpid, werr := syscall.Wait4(pid, &ws, syscall.WNOHANG, nil)
	if werr != nil {
		t.Fatalf("Wait4 liveness probe (pid=%d): %v", pid, werr)
	}
	if wpid == pid {
		t.Fatalf("worker process exited (%v) when the turn ctx was canceled — warm-pool "+
			"reuse is defeated; spawnWorker must not bind process lifetime to the turn ctx", ws)
	}
	// wpid == 0: the process is still running (not reaped) — correct.
}

// Startup failure must reap the exact temporary child, not just signal it.
func TestSpawnWorkerStartupFailureReapsExactChild(t *testing.T) {
	for _, mode := range []string{"cancel", "exit"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "child-pid")
			script := fmt.Sprintf("#!/bin/sh\necho $$ > %q\n", marker)
			if mode == "exit" {
				script += "exit 7\n"
			} else {
				script += "exec sleep 30\n"
			}
			if err := os.WriteFile(filepath.Join(dir, "cercano"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := spawnWorkerWithPolicy(ctx, "startup-failure", 1, SpawnPolicy{dir: dir}); done <- err }()
			var pid int
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				b, _ := os.ReadFile(marker)
				pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
				if pid > 0 {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if pid <= 0 {
				cancel()
				t.Fatalf("temporary child did not start: %v", <-done)
			}
			if mode == "cancel" {
				cancel()
			}
			if err := <-done; err == nil {
				t.Fatal("startup unexpectedly succeeded")
			}
			// Signal zero succeeds for a zombie; ESRCH therefore proves the child
			// has disappeared, rather than merely having received SIGKILL.
			deadline = time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				if syscall.Kill(pid, 0) == syscall.ESRCH {
					return
				}
				time.Sleep(time.Millisecond)
			}
			t.Fatalf("exact startup child %d remains live or unreaped", pid)
		})
	}
}
