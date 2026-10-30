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
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func runUpgradeWorker() int {
	root := os.Getenv("CERCANO_COORD_ROOT")
	for i := 0; ; i++ {
		if _, err := os.Stat(filepath.Join(root, "cancel-worker")); err == nil {
			return 0
		}
		if err := os.WriteFile(filepath.Join(root, "worker-progress"), []byte(strconv.Itoa(i)), 0600); err != nil {
			return 2
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Native external shutdown regression; this does not qualify migration acceptance.
func TestUpgradeStopsRealHostAndActiveWorker(t *testing.T) {
	testUpgradeStop(t, false)
}

func TestUpgradeCancellationAfterTERMFinishesCleanup(t *testing.T) {
	testUpgradeStop(t, true)
}

func testUpgradeStop(t *testing.T, cancelAfterTERM bool) {
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "Cellar", "cercano", "1", "bin", "cercano")
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, contents, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := netip.MustParseAddrPort(listener.Addr().String())
	listener.Close()
	host := exec.Command(binary, "agent")
	host.Env = []string{"HOME=" + root, "TMPDIR=" + root, "CERCANO_COORD_FIXTURE=1", "CERCANO_UPGRADE_WORKER=1", "CERCANO_COORD_ROOT=" + root, "CERCANO_COORD_ENDPOINT=" + endpoint.String()}
	if err := host.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = host.Wait(); close(done) }()
	hostIdentity, err := Inspect(host.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = signalIdentity(hostIdentity, syscall.SIGKILL); <-done })
	waitFixtureFile(t, filepath.Join(root, "1-ready"))
	pid, err := strconv.Atoi(string(waitFixtureFile(t, filepath.Join(root, "worker-pid"))))
	if err != nil {
		t.Fatal(err)
	}
	worker, err := Inspect(pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = signalIdentity(worker, syscall.SIGKILL) })
	before := string(waitFixtureFile(t, filepath.Join(root, "worker-progress")))
	time.Sleep(80 * time.Millisecond)
	after := string(waitFixtureFile(t, filepath.Join(root, "worker-progress")))
	if before == after {
		t.Fatal("worker not active")
	}
	canceled, cancelBefore := context.WithTimeout(context.Background(), time.Second)
	cancelBefore()
	if err := stopUpgradeHost(canceled, endpoint, []int{host.Process.Pid}); !errors.Is(err, context.Canceled) {
		t.Fatalf("already canceled: %v", err)
	}
	if now, err := Inspect(host.Process.Pid); err != nil || !now.SameProcess(hostIdentity) {
		t.Fatalf("already canceled shutdown disturbed host: %v", err)
	}
	progress := string(waitFixtureFile(t, filepath.Join(root, "worker-progress")))
	time.Sleep(80 * time.Millisecond)
	if progress == string(waitFixtureFile(t, filepath.Join(root, "worker-progress"))) {
		t.Fatal("already canceled shutdown disturbed worker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// An orphan candidate cannot authorize migration or disturb the host.
	if err := stopUpgradeHost(ctx, endpoint, []int{pid}); err == nil || !strings.Contains(err.Error(), "association") {
		t.Fatalf("orphan blocker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cancel-worker")); err == nil {
		t.Fatal("preflight disturbed worker")
	}
	// Only fixture PIDs enter discovery; native kernel checks and the actual
	// external signals control both disposable processes, not injected stubs.
	if cancelAfterTERM {
		// The worker exits on TERM. Cancel only after observing that irreversible
		// boundary, while the frozen host is still in its escalation interval.
		go func() {
			for {
				if _, err := Inspect(pid); errors.Is(err, syscall.ESRCH) {
					cancel()
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Millisecond):
				}
			}
		}()
	}
	err = stopUpgradeHost(ctx, endpoint, []int{host.Process.Pid})
	if cancelAfterTERM {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(host.Process.Pid); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("host exit not confirmed: %v", err)
	}
	if _, err := Inspect(pid); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("worker exit not confirmed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cancel-worker")); err == nil {
		t.Fatal("unexpected historical RPC shutdown")
	}
}
