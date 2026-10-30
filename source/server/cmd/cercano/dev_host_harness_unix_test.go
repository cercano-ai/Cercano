//go:build unix

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func devHostDump(cmd *exec.Cmd) { _ = cmd.Process.Signal(syscall.SIGQUIT) }

func devHostBoundBuild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

func devHostWorkersGone(t *testing.T, root string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "worker-*.pid"))
	if err != nil {
		t.Error(err)
		return
	}
	for _, path := range paths {
		var pid int
		if _, err := fmt.Sscanf(filepath.Base(path), "worker-%d.pid", &pid); err != nil || pid <= 0 {
			t.Errorf("invalid owned worker receipt")
			continue
		}
		deadline := time.Now().Add(3 * time.Second)
		for syscall.Kill(pid, 0) != syscall.ESRCH && time.Now().Before(deadline) {
			time.Sleep(25 * time.Millisecond)
		}
		if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
			t.Errorf("owned worker %d still present: %v", pid, err)
		} else {
			t.Logf("owned worker %d absent after teardown", pid)
		}
	}
}
