//go:build darwin && cgo

package brewrestart

import (
	"bufio"
	"errors"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// Reproduce proc_pidpath ENOENT with an existing saved launch path without
// reading the ambient process inventory. A dangling symlink is still an existing launch-path entry.
func replacedPathCandidate(t *testing.T, name string) (int, netip.AddrPort) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	alias := path + "-alias"
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(alias, "-test.run=^TestMissingExecutableChild$")
	child.Env = append(os.Environ(), "CERCANO_MISSING_CHILD=1")
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() {
		t.Fatal("helper not ready")
	}
	endpoint := netip.MustParseAddrPort(scanner.Text())
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(child.Process.Pid); !errors.Is(err, syscall.ENOENT) {
		t.Fatalf("discrepancy not reproduced: %v", err)
	}
	if _, err := os.Lstat(alias); err != nil {
		t.Fatal(err)
	}
	return child.Process.Pid, endpoint
}

func TestSurvivingAliasConcreteWriterRisks(t *testing.T) {
	for _, name := range []string{"cercano", "unrelated-helper"} {
		t.Run(name, func(t *testing.T) {
			pid, endpoint := replacedPathCandidate(t, name)
			if name == "cercano" {
				endpoint = netip.MustParseAddrPort("127.0.0.1:1")
			}
			host, err := discoverUpgradeHost(endpoint, []int{pid})
			// bf00a634 deliberately removed basename-only missing-image
			// blockers; concrete target-listener ownership still refuses.
			if name == "cercano" {
				if err != nil || host != nil {
					t.Fatalf("non-target alias: host=%v err=%v", host, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "uninspectable executable owns target listener; shutdown identity unresolved") {
				t.Fatalf("target-listener ambiguity not refused: %v", err)
			}
		})
	}
}
