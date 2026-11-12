//go:build darwin && cgo

package brewrestart

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestSignalIdentityReuseAtDelivery(t *testing.T) {
	id := Identity{PID: 42, StartSeconds: 100}
	replacementSignaled := false
	err := signalIdentityWith(id, syscall.SIGTERM,
		func(int) (Identity, error) { return id, nil },
		func(int) (uint32, error) { return 11, nil },
		func(pid int, version uint32, sig syscall.Signal) error {
			// Reuse occurs AFTER the last userspace check, at final delivery.
			// Version zero models the old PID-only kill (no version constraint).
			if version == 0 || version == 12 {
				replacementSignaled = true
				return nil
			}
			if version != 11 {
				t.Fatalf("unexpected captured version %d", version)
			}
			return syscall.ESRCH
		})
	if replacementSignaled {
		t.Fatal("PID-only delivery signaled replacement")
	}
	if !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("got %v, want stale-token ESRCH", err)
	}
}

func TestSignalIdentityBindingFailures(t *testing.T) {
	for _, name := range []string{"first_read", "inspection", "mismatch", "second_read", "version_changed", "missing_binding", "delivery"} {
		t.Run(name, func(t *testing.T) {
			id := Identity{PID: 42, StartSeconds: 100}
			reads, sends := 0, 0
			version := func(int) (uint32, error) {
				reads++
				if name == "first_read" || (name == "second_read" && reads == 2) {
					return 0, syscall.EIO
				}
				if name == "version_changed" && reads == 2 {
					return 12, nil
				}
				return 11, nil
			}
			if name == "missing_binding" {
				version = nil
			}
			err := signalIdentityWith(id, syscall.SIGKILL, func(int) (Identity, error) {
				if name == "inspection" {
					return Identity{}, syscall.ESRCH
				}
				if name == "mismatch" {
					return Identity{PID: 42, StartSeconds: 101}, nil
				}
				return id, nil
			}, version, func(pid int, v uint32, sig syscall.Signal) error {
				sends++
				if pid != 42 || v != 11 || sig != syscall.SIGKILL {
					t.Fatal("wrong delivery binding")
				}
				return syscall.EPERM
			})
			if err == nil {
				t.Fatal("expected refusal")
			}
			want := 0
			if name == "delivery" {
				want = 1
				if !errors.Is(err, syscall.EPERM) {
					t.Fatalf("lost errno: %v", err)
				}
			}
			if sends != want {
				t.Fatalf("sent %d times, want %d", sends, want)
			}
		})
	}
}

// Only this test's own subprocesses receive signals. A stale token must be
// rejected by the real kernel before the valid production path terminates it.
func TestSignalIdentityKernelBinding(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		t.Run(sig.String(), func(t *testing.T) {
			cmd := exec.Command("/bin/sleep", "30")
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			defer func() { _ = cmd.Process.Kill() }()
			id, err := Inspect(cmd.Process.Pid)
			if err != nil {
				t.Fatal(err)
			}
			version, err := processVersion(id.PID)
			if err != nil {
				t.Fatal(err)
			}
			if err := signalProcessVersion(id.PID, version+1, sig); !errors.Is(err, syscall.ESRCH) {
				t.Fatalf("stale token: got %v, want ESRCH", err)
			}
			select {
			case err := <-done:
				t.Fatalf("stale token terminated child: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			if err := signalIdentity(id, sig); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatalf("expected signal exit, got %v", err)
				}
				status := exit.Sys().(syscall.WaitStatus)
				if !status.Signaled() || status.Signal() != sig {
					t.Fatalf("exit status %v", status)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("bound signal did not terminate child")
			}
		})
	}
}
