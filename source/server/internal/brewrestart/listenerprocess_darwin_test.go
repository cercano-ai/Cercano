//go:build darwin && cgo

package brewrestart

import (
	"net"
	"net/netip"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestListenerProcessFindsThisProcess(t *testing.T) {
	for _, network := range []string{"127.0.0.1:0", ":0"} {
		ln, err := net.Listen("tcp", network)
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		id, found, err := ListenerProcess(netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port)))
		ln.Close()
		if err != nil || !found || id.PID != os.Getpid() {
			t.Fatalf("%s: ListenerProcess = %+v, %v, %v; want this process", network, id, found, err)
		}
		exited, err := ProcessExited(id)
		if err != nil || exited {
			t.Fatalf("ProcessExited(self) = %v, %v", exited, err)
		}
	}
}

func TestListenerProcessNoneForClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	if id, found, err := ListenerProcess(netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))); err != nil || found {
		t.Fatalf("ListenerProcess = %+v, %v, %v; want none", id, found, err)
	}
}

func TestProcessExitedForReapedAndZombieChildren(t *testing.T) {
	for _, reap := range []bool{true, false} {
		cmd := exec.Command("/bin/sleep", "30")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		id, err := Inspect(cmd.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		if exited, err := ProcessExited(id); err != nil || exited {
			t.Fatalf("running child: exited = %v, %v", exited, err)
		}
		_ = cmd.Process.Signal(syscall.SIGKILL)
		if reap {
			_ = cmd.Wait()
		} else {
			// Unreaped: the child is a zombie and holds no files.
			time.Sleep(200 * time.Millisecond)
		}
		exited, err := ProcessExited(id)
		if err != nil || !exited {
			t.Fatalf("reap=%v: exited = %v, %v; want true", reap, exited, err)
		}
		if !reap {
			_ = cmd.Wait()
		}
	}
}
