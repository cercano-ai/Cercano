//go:build darwin && cgo

package brewrestart

import (
	"context"
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

// Only the disposable child's PID/listener is signalled; no live host or DB.
func TestConfirmedLegacyTerminationReleasesLock(t *testing.T) {
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "1", "bin", "cercano")
	if err = os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(binary, data, 0700); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := netip.MustParseAddrPort(l.Addr().String())
	l.Close()
	lockPath := filepath.Join(root, "owner.lock")
	child := exec.Command(binary, "agent", "--host=fixture-host")
	child.Env = []string{"HOME=" + root, "TMPDIR=" + root, "CERCANO_COORD_FIXTURE=1", "CERCANO_COORD_ROOT=" + root, "CERCANO_COORD_ENDPOINT=" + endpoint.String(), "CERCANO_TERM_MODE=ignore", "CERCANO_OWNER_LOCK=" + lockPath}
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = child.Wait(); close(done) }()
	id, err := Inspect(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = signalIdentity(id, syscall.SIGKILL); <-done })
	waitFixtureFile(t, filepath.Join(root, "1-ready"))
	lock, err := os.OpenFile(lockPath, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		t.Fatal("fixture did not hold ownership lock")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	stale := id
	stale.StartSeconds++
	if err = StopLegacyHost(ctx, "fixture-host", endpoint, stale, func() error { return nil }); err == nil {
		t.Fatal("stale process approved")
	}
	if now, e := Inspect(id.PID); e != nil || !now.SameProcess(id) {
		t.Fatal("stale approval signalled process")
	}
	checks := 0
	start := time.Now()
	if err = StopLegacyHost(ctx, "fixture-host", endpoint, id, func() error { checks++; return nil }); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 2*time.Second || checks < 3 {
		t.Fatalf("missing bounded escalation/revalidation: %v checks=%d", time.Since(start), checks)
	}
	if _, err = Inspect(id.PID); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("death not verified: %v", err)
	}
	for {
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("owner lock retained: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
}
