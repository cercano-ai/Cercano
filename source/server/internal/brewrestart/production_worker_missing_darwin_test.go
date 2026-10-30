//go:build darwin && cgo

package brewrestart

import (
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Builds the real, untagged production entrypoint; no host/config/DB is opened.
func TestProductionWorkerMissingExecutable(t *testing.T) {
	root, err := os.MkdirTemp("", "cw-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
		if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("fixture directory remains: %v", err)
		}
	}()
	binary := filepath.Join(root, "cercano")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/cercano")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	socket := filepath.Join(root, "w.sock")
	child := exec.Command(binary, "worker", "--socket", socket)
	child.Env = append(os.Environ(), "HOME="+root)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
		if err := syscall.Kill(child.Process.Pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Errorf("owned child still exists: %v", err)
		} else {
			t.Logf("owned worker %d reaped and absent", child.Process.Pid)
		}
	}()
	deadline := time.Now().Add(15 * time.Second)
	for {
		c, err := net.DialTimeout("unix", socket, 100*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(child.Process.Pid); !errors.Is(err, syscall.ENOENT) {
		t.Fatalf("not missing: %v", err)
	}
	evidence, err := missingExecutableEvidence(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"lifetime", "saved-path", "uid"} {
		changed := evidence
		switch field {
		case "lifetime":
			changed.StartMicroseconds++
		case "saved-path":
			changed.Executable += "-changed"
		case "uid":
			changed.UID++
		}
		if _, err := holdsListenerInspected(changed, netip.MustParseAddrPort("127.0.0.1:1"), missingExecutableEvidence); err == nil {
			t.Fatalf("changed %s accepted", field)
		}
	}
	id, err := inspectUpgradeCandidate(child.Process.Pid, netip.MustParseAddrPort("127.0.0.1:1"))
	if err != nil || id.PID != 0 {
		t.Fatalf("production worker must be excluded: id=%+v err=%v", id, err)
	}
}

func TestMissingWorkerRoleEvidence(t *testing.T) {
	raw := func(args ...string) []byte {
		b := make([]byte, 4)
		binary.NativeEndian.PutUint32(b, uint32(len(args)))
		b = append(b, []byte("/gone/cercano\x00\x00")...)
		for _, a := range args {
			b = append(b, []byte(a)...)
			b = append(b, 0)
		}
		return b
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"worker", []string{"ignored", "worker", "--socket", "/tmp/w.sock"}, "/tmp/w.sock"},
		{"agent", []string{"ignored", "agent", "--socket", "/tmp/w.sock"}, ""},
		{"model-host", []string{"ignored", "model-host", "--socket", "/tmp/w.sock"}, ""},
		{"ambiguous", []string{"ignored", "worker"}, ""},
		{"relative", []string{"ignored", "worker", "--socket", "w.sock"}, ""},
		{"extra", []string{"ignored", "worker", "--socket", "/tmp/w.sock", "agent"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingWorkerSocket(raw(tc.args...)); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	for _, b := range [][]byte{nil, {4, 0, 0, 0}, raw("ignored", "worker", "--socket", "/tmp/w.sock")[:12]} {
		if missingWorkerSocket(b) != "" {
			t.Fatal("malformed args accepted")
		}
	}
	for _, tc := range []struct{ matched, listener, ok bool }{{true, false, true}, {false, false, false}, {true, true, false}, {false, true, false}} {
		if (missingWorkerSocketResult(tc.matched, tc.listener) == nil) != tc.ok {
			t.Fatalf("socket evidence %+v", tc)
		}
	}
}
