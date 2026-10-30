//go:build darwin && cgo

package brewrestart

import (
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"syscall"
	"testing"
)

func TestInspectionDisappearanceClassification(t *testing.T) {
	for _, tt := range []struct {
		name                string
		initial, next, want syscall.Errno
		probes              int
	}{
		{"confirmed disappearance", syscall.ENOENT, syscall.ESRCH, syscall.ESRCH, 1},
		{"live missing executable", syscall.ENOENT, 0, syscall.ENOENT, 1},
		{"permission on recheck", syscall.ENOENT, syscall.EPERM, syscall.ENOENT, 1},
		{"ambiguous recheck", syscall.ENOENT, syscall.EAGAIN, syscall.ENOENT, 1},
		{"missing path again", syscall.ENOENT, syscall.ENOENT, syscall.ENOENT, 1},
		{"initial permission", syscall.EPERM, syscall.ESRCH, syscall.EPERM, 0},
		{"initial access denied", syscall.EACCES, syscall.ESRCH, syscall.EACCES, 0},
		{"initial unstable identity", syscall.EAGAIN, syscall.ESRCH, syscall.EAGAIN, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			err := confirmedInspectionError(tt.initial, func() syscall.Errno { calls++; return tt.next })
			if !errors.Is(err, tt.want) || calls != tt.probes {
				t.Fatalf("error %v probes %d", err, calls)
			}
			f := &fakeProcesses{pids: []int{283}, inspectErrors: map[int]error{283: fmt.Errorf("inspect PID 283: %w", err)}}
			owner, discoverErr := discover(f, "/opt/homebrew/Cellar/cercano/2/bin/cercano", 501, netip.MustParseAddrPort("127.0.0.1:12345"))
			if owner != nil || (discoverErr == nil) != (tt.want == syscall.ESRCH) || len(f.probed) != 0 {
				t.Fatalf("owner %v error %v probes %v", owner, discoverErr, f.probed)
			}
		})
	}
}

func TestInspectReapedDisposableProcess(t *testing.T) {
	cmd := exec.Command("/usr/bin/true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(cmd.Process.Pid); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("reaped child: %v", err)
	}
}
