//go:build darwin && cgo

package brewrestart

import (
	"errors"
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

func TestUpgradeIgnoresUnidentifiedUnrelatedProcess(t *testing.T) {
	root := t.TempDir()
	image := filepath.Join(root, "cercano")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(image, data, 0700); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(image)
	child.Env = []string{"HOME=" + root, "CERCANO_TREE_FIXTURE=1"}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
		if err := syscall.Kill(child.Process.Pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Errorf("fixture not reaped: %v", err)
		}
	}()
	time.Sleep(100 * time.Millisecond)
	if err := os.Remove(image); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(child.Process.Pid); !errors.Is(err, syscall.ENOENT) {
		t.Fatalf("expected actual missing-image probe: %v", err)
	}
	targets, err := discoverUpgradeTargets(netip.MustParseAddrPort("127.0.0.1:1"), []int{child.Process.Pid}, false)
	if err != nil || len(targets) != 0 {
		t.Errorf("unrelated process blocked discovery: targets=%v err=%v", targets, err)
	}
	if !IsUpgradeUnidentifiedOrNonWriter(child.Process.Pid) {
		t.Error("unrelated unidentified process blocks post-stop gate")
	}
	if err := syscall.Kill(child.Process.Pid, 0); err != nil {
		t.Fatalf("unrelated process was terminated: %v", err)
	}
}

// Only the owned fixture PID is inspected. No enumeration or upgrade signals.
func TestUpgradeRefusesUninspectableTargetListener(t *testing.T) {
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(root, "1", "bin", "cercano")
	if err := os.MkdirAll(filepath.Dir(image), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(image, data, 0700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := listener.Addr().String()
	listener.Close()
	child := exec.Command(image, "agent")
	child.Env = []string{"HOME=" + root, "CERCANO_COORD_FIXTURE=1", "CERCANO_COORD_ROOT=" + root, "CERCANO_COORD_ENDPOINT=" + endpoint}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	waitFixtureFile(t, filepath.Join(root, "1-ready"))
	t.Run("identified-surviving-host-remains-blocker", func(t *testing.T) {
		if IsUpgradeUnidentifiedOrNonWriter(child.Process.Pid) {
			t.Fatal("identified surviving host excluded from post-stop gate")
		}
	})
	if err := os.Remove(image); err != nil {
		t.Fatal(err)
	}
	for _, aggregate := range []bool{false, true} {
		_, err := discoverUpgradeTargets(netip.MustParseAddrPort(endpoint), []int{child.Process.Pid}, aggregate)
		if err == nil || !strings.Contains(err.Error(), "owns target listener") {
			t.Fatalf("target ambiguity not refused: %v", err)
		}
	}
	if err := syscall.Kill(child.Process.Pid, 0); err != nil {
		t.Fatalf("ambiguous target signalled: %v", err)
	}
}
