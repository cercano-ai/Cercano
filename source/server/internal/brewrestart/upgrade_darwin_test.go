//go:build darwin && cgo

package brewrestart

import (
	"context"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

func TestUpgradeStopsRealHostAndActiveWorker(t *testing.T) {
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
	t.Cleanup(func() { _ = host.Process.Kill(); <-done })
	waitFixtureFile(t, filepath.Join(root, "1-ready"))
	pid, err := strconv.Atoi(string(waitFixtureFile(t, filepath.Join(root, "worker-pid"))))
	if err != nil {
		t.Fatal(err)
	}
	worker, err := Inspect(pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if now, err := Inspect(pid); err == nil && now.SameProcess(worker) {
			p, _ := os.FindProcess(pid)
			_ = p.Kill()
		}
	})
	before := string(waitFixtureFile(t, filepath.Join(root, "worker-progress")))
	time.Sleep(80 * time.Millisecond)
	after := string(waitFixtureFile(t, filepath.Join(root, "worker-progress")))
	if before == after {
		t.Fatal("worker not active")
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
	// shutdown RPC control both disposable processes, not injected RPC stubs.
	if err := stopUpgradeHost(ctx, endpoint, []int{host.Process.Pid, pid}); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(host.Process.Pid); err == nil {
		t.Fatal("host survived")
	}
	if _, err := Inspect(pid); err == nil {
		t.Fatal("worker survived")
	}
	if _, err := os.Stat(filepath.Join(root, "cancel-worker")); err != nil {
		t.Fatal("worker cancellation missing", err)
	}
}
