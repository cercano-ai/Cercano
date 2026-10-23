//go:build darwin && cgo

package brewrestart

import (
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// runGoneFixture listens on loopback, writes its port to
// CERCANO_GONE_PORT_FILE, and waits to be killed.
func runGoneFixture() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 2
	}
	defer ln.Close()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	if err := os.WriteFile(os.Getenv("CERCANO_GONE_PORT_FILE"), []byte(port), 0o600); err != nil {
		return 2
	}
	time.Sleep(30 * time.Second)
	return 0
}

// startCercanoCopy runs a copy of this test binary named cercano from
// <root>/keg/bin and returns it with the path it runs from and its port.
func startCercanoCopy(t *testing.T) (*exec.Cmd, string, int) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "keg", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(bin, "cercano")
	copyExecutable(t, exe)
	portFile := filepath.Join(root, "port")
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "CERCANO_GONE_FIXTURE=1", "CERCANO_GONE_PORT_FILE="+portFile)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, err := os.ReadFile(portFile)
		if port, perr := strconv.Atoi(string(data)); err == nil && perr == nil && port > 0 {
			return cmd, exe, port
		}
		if time.Now().After(deadline) {
			t.Fatal("the cercano copy did not start listening")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func copyExecutable(t *testing.T, dst string) {
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

func findListed(t *testing.T, pid int, details func(UserProcess) bool) (UserProcess, bool) {
	t.Helper()
	procs, err := ListUserProcesses(details)
	if err != nil {
		t.Fatalf("ListUserProcesses: %v", err)
	}
	for _, p := range procs {
		if p.PID == pid {
			return p, true
		}
	}
	return UserProcess{}, false
}

// Homebrew's cleanup deletes the old keg while an old `cercano --mcp` still
// runs from it; the kernel then has no path for its executable. The process
// must still be listed, by name, with its executable marked gone.
func TestListUserProcessesKeepsProcessWhoseExecutableWasRemoved(t *testing.T) {
	before := time.Now().Add(-time.Second)
	cmd, exe, _ := startCercanoCopy(t)
	pid := cmd.Process.Pid

	p, ok := findListed(t, pid, nil)
	if !ok || p.Executable != exe || p.ExecutableGone {
		t.Fatalf("before removal: listed %v as %+v; want executable %q", ok, p, exe)
	}

	if err := os.RemoveAll(filepath.Dir(filepath.Dir(exe))); err != nil {
		t.Fatal(err)
	}
	var asked []UserProcess
	p, ok = findListed(t, pid, func(p UserProcess) bool {
		asked = append(asked, p)
		return p.Name == "cercano"
	})
	if !ok {
		t.Fatalf("the cercano process (%d) whose executable was removed is not listed", pid)
	}
	if !p.ExecutableGone || p.Executable != "" || p.Name != "cercano" {
		t.Fatalf("listed as %+v; want ExecutableGone with Name cercano", p)
	}
	if p.Started.Before(before) || p.Started.After(time.Now()) {
		t.Errorf("Started = %v, want between %v and now", p.Started, before)
	}
	if len(p.Args) == 0 || filepath.Base(p.Args[0]) != "cercano" || !p.HomeKnown {
		t.Errorf("details not read: Args %q, HomeKnown %v", p.Args, p.HomeKnown)
	}
	found := false
	for _, a := range asked {
		found = found || a.PID == pid && a.ExecutableGone && a.Name == "cercano"
	}
	if !found {
		t.Error("details was not asked about the process whose executable is gone")
	}
}

// ListenerProcess must resolve a listener whose executable was removed, so
// migration can recognise and wait for a pre-profile agent from a deleted
// keg.
func TestListenerProcessFindsHolderWhoseExecutableWasRemoved(t *testing.T) {
	cmd, exe, port := startCercanoCopy(t)
	if err := os.RemoveAll(filepath.Dir(filepath.Dir(exe))); err != nil {
		t.Fatal(err)
	}
	endpoint := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))
	id, found, err := ListenerProcess(endpoint)
	if err != nil || !found || id.PID != cmd.Process.Pid {
		t.Fatalf("ListenerProcess = %+v, %v, %v; want PID %d", id, found, err, cmd.Process.Pid)
	}
	if !id.ExecutableGone || id.Name != "cercano" || id.Executable != "" || id.StartSeconds == 0 {
		t.Fatalf("identity = %+v; want ExecutableGone with Name cercano and a start time", id)
	}
	if holds, err := HoldsListener(id, endpoint); err != nil || !holds {
		t.Fatalf("HoldsListener = %v, %v; want true", holds, err)
	}
	if exited, err := ProcessExited(id); err != nil || exited {
		t.Fatalf("ProcessExited while running = %v, %v", exited, err)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if exited, err := ProcessExited(id); err != nil || !exited {
		t.Fatalf("ProcessExited after kill = %v, %v", exited, err)
	}
	// Inspect keeps requiring a path: the restart coordinator relies on it.
	if _, err := Inspect(os.Getpid()); err != nil {
		t.Fatalf("Inspect(self): %v", err)
	}
}
