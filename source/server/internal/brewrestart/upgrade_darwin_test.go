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
	var afterTERM func()
	if cancelAfterTERM {
		// Producers now remain frozen through escalation, including workers.
		// Observe completed native TERM delivery, not worker exit (which now
		// occurs during KILL). No signals or exit checks are mocked.
		afterTERM = cancel
	}
	candidates := []int{host.Process.Pid}
	unrelated := 0
	if !cancelAfterTERM {
		unrelated, _ = replacedPathCandidate(t, "unrelated-helper")
		candidates = append(candidates, unrelated)
	}
	err = stopUpgradeHostObserved(ctx, endpoint, candidates, afterTERM)
	if cancelAfterTERM {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	if unrelated != 0 {
		if err := missingIdentityError(unrelated); err != nil {
			t.Fatalf("excluded helper was disturbed: %v", err)
		}
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

// Discovery is fixture-PID scoped; executable, sockets, signals and exit checks
// are native. These are test-binary agents, not production provider acceptance.
func TestUpgradeMultipleActiveHosts(t *testing.T) {
	var pids []int
	var ids []Identity
	var endpoints []netip.AddrPort
	for i := 0; i < 2; i++ {
		root := t.TempDir()
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(exe)
		if err != nil {
			t.Fatal(err)
		}
		binary := filepath.Join(root, "Cellar", "cercano", "1", "bin", "cercano")
		if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binary, data, 0700); err != nil {
			t.Fatal(err)
		}
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		endpoint := netip.MustParseAddrPort(l.Addr().String())
		l.Close()
		host := exec.Command(binary, "agent")
		host.Env = []string{"HOME=" + root, "TMPDIR=" + root, "CERCANO_COORD_FIXTURE=1", "CERCANO_UPGRADE_WORKER=1", "CERCANO_COORD_ROOT=" + root, "CERCANO_COORD_ENDPOINT=" + endpoint.String()}
		if err := host.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { _ = host.Wait(); close(done) }()
		id, err := Inspect(host.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = signalIdentity(id, syscall.SIGKILL); <-done })
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
		if before == string(waitFixtureFile(t, filepath.Join(root, "worker-progress"))) {
			t.Fatal("worker not active")
		}
		pids = append(pids, id.PID, worker.PID)
		ids = append(ids, id, worker)
		endpoints = append(endpoints, endpoint)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// A stale identity for the additional host must not authorize any signal.
	unsafe := ids[2]
	unsafe.StartSeconds++
	if err := stopVerifiedUpgradeHost(ctx, upgradeTarget{unsafe, endpoints[1]}, nil); err == nil {
		t.Fatal("unsafe additional host identity accepted")
	}
	for _, id := range ids {
		current, err := Inspect(id.PID)
		if err != nil || !id.SameProcess(current) {
			t.Fatalf("unsafe refusal disrupted target %d: %v", id.PID, err)
		}
	}
	if err := stopUpgradeHost(ctx, endpoints[0], pids); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, err := Inspect(id.PID); !errors.Is(err, syscall.ESRCH) {
			t.Fatalf("target %d survived: %v", id.PID, err)
		}
	}
}
