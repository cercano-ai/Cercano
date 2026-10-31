//go:build darwin && cgo

package brewrestart

import (
	"context"

	"cercano/source/server/internal/conversation"
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

func TestUpgradeStopsDifferentImageWorker(t *testing.T) {
	for _, name := range []string{"cercano", "cercano-agent", "unrecognized-worker"} {
		t.Run(name, func(t *testing.T) { testUpgradeStopWithWorker(t, false, name) })
	}
}

func testUpgradeStop(t *testing.T, cancelAfterTERM bool) {
	testUpgradeStopWithWorker(t, cancelAfterTERM, "")
}

func testUpgradeStopWithWorker(t *testing.T, cancelAfterTERM bool, workerName string, childRoles ...string) {
	childRole := "worker"
	if len(childRoles) > 0 {
		childRole = childRoles[0]
	}
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
	host.Env = append(host.Env, "CERCANO_COORD_CHILD_ROLE="+childRole)
	if workerName != "" {
		workerBinary := filepath.Join(root, "worker-version", workerName)
		if err := os.MkdirAll(filepath.Dir(workerBinary), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(workerBinary, contents, 0700); err != nil {
			t.Fatal(err)
		}
		host.Env = append(host.Env, "CERCANO_COORD_WORKER_BINARY="+workerBinary)
	}
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
	if workerName != "" {
		role, roleErr := processRole(worker)
		if roleErr != nil || role != childRole || worker.Executable == hostIdentity.Executable || worker.ParentPID != hostIdentity.PID || worker.UID != hostIdentity.UID {
			t.Fatalf("different-image probe: host=%+v worker=%+v role=%q err=%v", hostIdentity, worker, role, roleErr)
		}
		t.Logf("verified attached same-user child role=%s with different executable path", safeUpgradeRole(role))
	}
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
	timeout := 5 * time.Second
	if workerName == "unrecognized-worker" || childRole != "worker" {
		timeout = 500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// A recognized worker is not a database writer, even without its host.
	if childRole == "worker" {
		if err := stopUpgradeHost(ctx, endpoint, []int{pid}); err != nil {
			t.Fatalf("recognized worker blocked discovery: %v", err)
		}
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
	if workerName == "unrecognized-worker" || childRole != "worker" {
		t.Logf("probe: child pid=%d role=%s image=%s; stop error=%v", pid, safeUpgradeRole(childRole), workerName, err)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unrecognized image must block: %v", err)
		}
		for _, fragment := range []string{"pid=" + strconv.Itoa(pid), "role=" + safeUpgradeRole(childRole), "unresolved=1 omitted=0"} {
			if !strings.Contains(err.Error(), fragment) {
				t.Fatalf("missing child diagnostic %q: %v", fragment, err)
			}
		}
		if strings.Contains(err.Error(), "fixture-secret") {
			t.Fatalf("child argument leaked: %v", err)
		}
		for _, id := range []Identity{hostIdentity, worker} {
			if now, err := Inspect(id.PID); err != nil || !now.SameProcess(id) {
				t.Fatalf("refusal disturbed fixture identity: %v", err)
			}
		}
		return
	}
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

// The worker is a sibling, not a descendant of the target host. It must
// neither prevent host shutdown nor acquire signal authority by exclusion.
func TestUpgradeHostWithUnrelatedRecognizedWorker(t *testing.T) {
	for _, role := range []string{"agent", "development-host"} {
		t.Run(role, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			db := filepath.Join(root, "history.db")
			store, err := conversation.Open(db)
			if err != nil {
				t.Fatal(err)
			}
			store.Close()
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(self)
			if err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(root, "1", "bin", "cercano")
			if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(binary, data, 0700); err != nil {
				t.Fatal(err)
			}
			worker := exec.Command(binary, "worker")
			worker.Env = append(os.Environ(), "CERCANO_UPGRADE_WORKER=1", "CERCANO_COORD_ROOT="+root)
			if err := worker.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = worker.Process.Kill(); _ = worker.Wait() }()
			waitFixtureFile(t, filepath.Join(root, "worker-progress"))
			wid, err := Inspect(worker.Process.Pid)
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			endpoint := netip.MustParseAddrPort(listener.Addr().String())
			listener.Close()
			host := exec.Command(binary, role)
			host.Env = []string{"HOME=" + root, "TMPDIR=" + root, "CERCANO_COORD_FIXTURE=1", "CERCANO_COORD_ROOT=" + root, "CERCANO_COORD_ENDPOINT=" + endpoint.String()}
			if err := host.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() { _ = host.Wait(); close(done) }()
			defer func() { _ = host.Process.Kill(); <-done }()
			waitFixtureFile(t, filepath.Join(root, "1-ready"))
			if wid.ParentPID == host.Process.Pid {
				t.Fatal("worker unexpectedly associated")
			}
			if !IsUpgradeUnidentifiedOrNonWriter(worker.Process.Pid) {
				t.Fatal("post-stop gate rejects recognized worker")
			}
			if IsUpgradeUnidentifiedOrNonWriter(host.Process.Pid) {
				t.Fatal("post-stop gate excludes host")
			}
			targets, err := discoverUpgradeTargets(netip.MustParseAddrPort("127.0.0.1:1"), []int{host.Process.Pid, worker.Process.Pid}, false)
			if err != nil || len(targets) != 1 || targets[0].PID != host.Process.Pid {
				t.Fatalf("additional %s host discovery: %v %v", role, targets, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := stopUpgradeHost(ctx, endpoint, []int{host.Process.Pid, worker.Process.Pid}); err != nil {
				t.Fatal(err)
			}
			if _, err := Inspect(host.Process.Pid); !errors.Is(err, syscall.ESRCH) {
				t.Fatalf("host exit not verified: %v", err)
			}
			// Disposable integrated sequence: only after native verified host exit,
			// check the surviving worker, save a backup, then migrate the store.
			if !IsUpgradeUnidentifiedOrNonWriter(worker.Process.Pid) {
				t.Fatal("worker blocks post-stop DB gate")
			}
			original, err := os.ReadFile(db)
			if err != nil {
				t.Fatal(err)
			}
			backup := db + ".backup"
			if err := os.WriteFile(backup, original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := conversation.UpgradeOffline(ctx, db); err != nil {
				t.Fatal(err)
			}
			upgraded, err := conversation.OpenCoexistence(ctx, db, conversation.DevelopmentCoexistenceDeclaration())
			if err != nil {
				t.Fatal(err)
			}
			upgraded.Close()
			t.Log("verified host exit -> non-writer check -> disposable backup -> migration")
			before := string(waitFixtureFile(t, filepath.Join(root, "worker-progress")))
			time.Sleep(80 * time.Millisecond)
			if before == string(waitFixtureFile(t, filepath.Join(root, "worker-progress"))) {
				t.Fatal("unrelated worker was stopped")
			}
			now, err := Inspect(worker.Process.Pid)
			if err != nil || !now.SameProcess(wid) {
				t.Fatalf("unrelated worker lost: %v", err)
			}
		})
	}
}

// These are real compiled child processes, not mocked role/identity reads.
func TestUpgradeUnknownChildRoles(t *testing.T) {
	for _, role := range []string{"agent", "development-host", "--fixture-secret=credential"} {
		t.Run(safeUpgradeRole(role), func(t *testing.T) { testUpgradeStopWithWorker(t, false, "cercano", role) })
	}
}
