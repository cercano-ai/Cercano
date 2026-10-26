//go:build darwin && cgo

package profilemigrate

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// copySelf copies this test binary to dst.
func copySelf(t *testing.T, dst string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// startSleeper runs exe as a sleeping fixture until the test ends. It
// returns once the fixture runs its own code: macOS kills a process whose
// executable is removed while it is still being loaded.
func startSleeper(t *testing.T, exe string) int {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(exe, "--mcp")
	cmd.Env = append(os.Environ(), "CERCANO_SLEEP_FIXTURE=1", "CERCANO_SLEEP_READY="+ready)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(ready); err == nil {
			return cmd.Process.Pid
		}
		if time.Now().After(deadline) {
			t.Fatalf("the fixture %s did not start", exe)
		}
	}
}

func listedProcess(procs []Process, pid int) (Process, bool) {
	for _, p := range procs {
		if p.PID == pid {
			return p, true
		}
	}
	return Process{}, false
}

func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// An older build that `go install`, the dev launcher or an installer
// replaced in place still reports the same path on macOS. It must be
// counted; a process started from the new file must not be.
func TestOldBuildReplacedInPlaceIsCounted(t *testing.T) {
	home := os.Getenv("HOME")
	dir := resolvedTempDir(t)
	exe := filepath.Join(dir, "cercano")
	copySelf(t, exe)
	oldPID := startSleeper(t, exe)

	time.Sleep(50 * time.Millisecond)
	next := filepath.Join(dir, "cercano.new")
	copySelf(t, next)
	if err := os.Rename(next, exe); err != nil {
		t.Fatal(err)
	}
	newPID := startSleeper(t, exe)

	self := os.Getpid()
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	// This process stands in for the new build running from exe.
	cur := program{Path: exe, ModTime: info.ModTime()}
	if got := currentProgram([]Process{{PID: self, Executable: exe}}, self); got.Path != cur.Path || !got.ModTime.Equal(cur.ModTime) {
		t.Errorf("currentProgram = %+v, want %+v", got, cur)
	}

	procs, err := listUserProcesses()
	if err != nil {
		t.Fatal(err)
	}
	old, ok := listedProcess(procs, oldPID)
	if !ok {
		t.Fatalf("old process %d not listed", oldPID)
	}
	if old.Executable != exe {
		t.Logf("old process reports %q after the replacement", old.Executable)
	}
	pids := otherCercanoPIDs(procs, self, home, cur)
	if !slices.Contains(pids, oldPID) {
		t.Errorf("otherCercanoPIDs = %v: the build replaced in place (pid %d, started %v, file written %v) is excluded as this build",
			pids, oldPID, old.Started, cur.ModTime)
	}
	if slices.Contains(pids, newPID) {
		t.Errorf("otherCercanoPIDs = %v: the process started from the new file (pid %d) is counted", pids, newPID)
	}
}

// A process whose keg was deleted is still counted, and never taken for
// this build even when this build ran from that same path.
func TestProcessWhoseExecutableWasRemovedIsCounted(t *testing.T) {
	home := os.Getenv("HOME")
	dir := resolvedTempDir(t)
	bin := filepath.Join(dir, "Cellar", "cercano", "0.9.0", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(bin, "cercano")
	copySelf(t, exe)
	pid := startSleeper(t, exe)
	if err := os.RemoveAll(filepath.Join(dir, "Cellar", "cercano", "0.9.0")); err != nil {
		t.Fatal(err)
	}

	procs, err := listUserProcesses()
	if err != nil {
		t.Fatal(err)
	}
	p, ok := listedProcess(procs, pid)
	if !ok {
		t.Fatalf("process %d whose executable was removed is not listed", pid)
	}
	if !p.ExecutableGone || p.Name != "cercano" {
		t.Errorf("listed as %+v, want ExecutableGone with Name cercano", p)
	}
	cur := program{Path: exe, ModTime: time.Now().Add(-time.Hour)}
	if pids := otherCercanoPIDs(procs, os.Getpid(), home, cur); !slices.Contains(pids, pid) {
		t.Fatalf("otherCercanoPIDs = %v, want it to include %d", pids, pid)
	}
}
