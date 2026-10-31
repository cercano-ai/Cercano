//go:build darwin && cgo

package brewrestart

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestMissingExecutableFinalInspection(t *testing.T) {
	for _, tc := range []struct {
		name            string
		input           error
		absent, allowed bool
	}{
		{"confirmed disappearance", fmt.Errorf("inspect: %w", syscall.ESRCH), true, false},
		{"still missing", syscall.ENOENT, false, true},
		{"successful inspection", nil, false, false},
		{"permission", syscall.EPERM, false, false},
		{"ambiguous", syscall.EAGAIN, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := missingExecutableStateError(tc.input)
			if (err == nil) != tc.allowed || errors.Is(err, syscall.ESRCH) != tc.absent {
				t.Fatalf("got %v; allowed=%v absent=%v", err, tc.allowed, tc.absent)
			}
		})
	}
}

func TestMissingListenerRequiresConfirmedDisappearance(t *testing.T) {
	for _, socketErr := range []error{syscall.ESRCH, syscall.EAGAIN, syscall.EPERM} {
		for _, identityErr := range []error{nil, syscall.ENOENT, syscall.EAGAIN, syscall.EPERM, syscall.ESRCH} {
			t.Run(fmt.Sprintf("socket=%v/identity=%v", socketErr, identityErr), func(t *testing.T) {
				calls := 0
				err := missingListenerError(42, socketErr, func(pid int) error {
					calls++
					if pid != 42 {
						t.Fatal(pid)
					}
					return identityErr
				})
				if err == nil || errors.Is(err, syscall.ESRCH) != errors.Is(identityErr, syscall.ESRCH) || calls != 1 {
					t.Fatalf("err=%v probe calls=%d", err, calls)
				}
			})
		}
	}
}

// This child has no Go runtime descriptors. EOF on stdout is the handshake:
// stdout is closed last, after every other descriptor, before sleeping.
func TestMissingExecutableZeroFDAndConfirmedExit(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "child.c")
	executable := filepath.Join(dir, "unrelated-zero-fd")
	if err := os.WriteFile(source, []byte(`#include <unistd.h>
int main(void) {
 for (int fd = getdtablesize()-1; fd >= 0; fd--) if (fd != 1) close(fd);
 close(1);
 sleep(60);
 return 0;
}
`), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cc", source, "-o", executable).CombinedOutput(); err != nil {
		t.Fatalf("compile child: %v: %s", err, out)
	}
	child := exec.Command(executable)
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	if _, err := io.ReadAll(output); err != nil {
		t.Fatal(err)
	}
	pid := child.Process.Pid
	if err := os.Remove(executable); err != nil {
		t.Fatal(err)
	}
	if err := missingIdentityError(pid); err != nil {
		t.Fatalf("child not live: %v", err)
	}
	endpoint := netip.MustParseAddrPort("127.0.0.1:1")
	// Discovery policy from bf00a634: unavailable images without concrete
	// target-listener ownership are irrelevant, not signal-authorized.
	if host, err := discoverUpgradeHost(endpoint, []int{pid}); err != nil || host != nil {
		t.Fatalf("unrelated live zero-FD candidate: host=%v err=%v", host, err)
	}
	if err := missingListenerError(pid, syscall.ESRCH, missingIdentityError); err == nil || errors.Is(err, syscall.ESRCH) {
		t.Fatalf("live ambiguous listener must refuse: %v", err)
	}
	_ = child.Process.Kill()
	_ = child.Wait()
	waited = true
	if err := missingIdentityError(pid); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("exit not confirmed: %v", err)
	}
	if err := missingExecutableStateError(missingIdentityError(pid)); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("lost confirmed disappearance: %v", err)
	}
	if err := missingListenerError(pid, syscall.EAGAIN, missingIdentityError); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("lost confirmed listener disappearance: %v", err)
	}
	if host, err := discoverUpgradeHost(endpoint, []int{pid}); err != nil || host != nil {
		t.Fatalf("confirmed gone candidate: host=%v err=%v", host, err)
	}
}
