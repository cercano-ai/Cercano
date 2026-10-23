//go:build linux

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
		if exited, err := ProcessExited(id); err != nil || exited {
			t.Fatalf("ProcessExited(self) = %v, %v", exited, err)
		}
	}
}

func TestProcessExitedForReapedAndZombieChildren(t *testing.T) {
	for _, reap := range []bool{true, false} {
		cmd := exec.Command("sleep", "30")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		id, err := procIdentity(cmd.Process.Pid, uint32(os.Getuid()))
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
			time.Sleep(200 * time.Millisecond)
		}
		if exited, err := ProcessExited(id); err != nil || !exited {
			t.Fatalf("reap=%v: exited = %v, %v; want true", reap, exited, err)
		}
		if !reap {
			_ = cmd.Wait()
		}
	}
}
