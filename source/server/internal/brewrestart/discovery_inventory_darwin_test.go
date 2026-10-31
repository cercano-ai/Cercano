//go:build darwin && cgo

package brewrestart

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMissingExecutableChild(t *testing.T) {
	if os.Getenv("CERCANO_MISSING_CHILD") != "1" {
		t.Skip("child only")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	fmt.Println(listener.Addr())
	time.Sleep(time.Minute)
}

// Enumeration is production ListCandidates, not a fixture implementation.
// Only disposable children are inspected/signalled; no database is opened.
func TestScopedUnlinkedExecutableCandidates(t *testing.T) {
	for _, tc := range []struct {
		name, executable, role string
		targetListener, refuse bool
	}{
		{"unrelated", "unrelated-sleep", "", false, false},
		{"historical-host", "cercano", "agent", false, false},
		{"symlink-host", "cercano", "agent", false, false},
		{"agent-host", "cercano-agent", "agent", false, false},
		{"model-host-unresolved", "cercano", "model-host", false, false},
		{"worker-unresolved", "cercano", "worker", false, false},
		{"ambiguous-target-listener", "unknown-host", "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.executable)
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(self)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0700); err != nil {
				t.Fatal(err)
			}
			launchPath := path
			if tc.name == "symlink-host" {
				launchPath = filepath.Join(filepath.Dir(path), "unrelated-alias")
				if err := os.Symlink(path, launchPath); err != nil {
					t.Fatal(err)
				}
			}
			child := exec.Command(launchPath, "-test.run=^TestMissingExecutableChild$", tc.role)
			child.Env = append(os.Environ(), "CERCANO_MISSING_CHILD=1")
			output, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
			scanner := bufio.NewScanner(output)
			if !scanner.Scan() {
				t.Fatal("not ready")
			}
			endpoint, err := netip.ParseAddrPort(scanner.Text())
			if err != nil {
				t.Fatal(err)
			}
			if tc.executable == "cercano" || tc.executable == "cercano-agent" {
				host, err := discoverUpgradeHost(endpoint, []int{child.Process.Pid})
				if err != nil || host == nil || host.PID != child.Process.Pid {
					t.Fatalf("intact disposable host discovery: host=%v err=%v", host, err)
				}
			}
			if !tc.targetListener {
				endpoint = netip.MustParseAddrPort("127.0.0.1:1")
			}
			if _, err := Inspect(child.Process.Pid); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if _, err := Inspect(child.Process.Pid); !errors.Is(err, syscall.ENOENT) {
				t.Fatalf("probe did not reproduce missing executable: %v", err)
			}
			_, err = discoverUpgradeHost(endpoint, []int{child.Process.Pid})
			t.Logf("production enumeration candidate classification: %v", err)
			if tc.refuse {
				if err == nil || !strings.Contains(err.Error(), "owns target listener") {
					t.Fatalf("lost specific refusal: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Missing unrelated images are excluded; intact blockers are diagnosed in one pass.
// Production retains its first refusal and neither path returns a stop identity.
func TestDiscoveryInventoryAggregatesWithoutChangingRefusal(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	var pids []int
	for i := 0; i < 3; i++ {
		path := filepath.Join(t.TempDir(), "cercano")
		if err := os.WriteFile(path, data, 0700); err != nil {
			t.Fatal(err)
		}
		child := exec.Command(path, "-test.run=^TestMissingExecutableChild$", "worker")
		child.Env = append(os.Environ(), "CERCANO_MISSING_CHILD=1")
		output, err := child.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
		if !bufio.NewScanner(output).Scan() {
			t.Fatal("child not ready")
		}
		pids = append(pids, child.Process.Pid)
		if i == 0 {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
	}
	endpoint := netip.MustParseAddrPort("127.0.0.1:1")
	host, first := discoverUpgradeHost(endpoint, pids)
	if host != nil || first == nil {
		t.Fatalf("production accepted: %v %v", host, first)
	}
	if !strings.Contains(first.Error(), fmt.Sprintf("pid=%d", pids[1])) || strings.Contains(first.Error(), fmt.Sprintf("pid=%d", pids[0])) {
		t.Fatalf("production did not retain first refusal: %v", first)
	}
	host, all := discoverUpgradeHostInventory(endpoint, pids, true)
	if host != nil || all == nil {
		t.Fatalf("inventory accepted: %v %v", host, all)
	}
	for _, pid := range pids[1:] {
		if !strings.Contains(all.Error(), fmt.Sprintf("pid=%d", pid)) {
			t.Fatalf("inventory omitted pid=%d: %v", pid, all)
		}
	}
	if !strings.Contains(all.Error(), first.Error()) || !strings.Contains(all.Error(), "unresolved possible writer role") {
		t.Fatalf("classification lost: %v", all)
	}
}
