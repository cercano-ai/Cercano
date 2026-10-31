//go:build darwin && cgo

package brewrestart

import (
	"net/netip"
	"os"
	"testing"
)

// Read-only supplemental target report. Never call stopUpgradeHost here.
// The standalone preflight retains authoritative full-inventory aggregated errors.
func TestFinalReadOnlyInventoryReport(t *testing.T) {
	if os.Getenv("FINAL_READ_ONLY_INVENTORY") != "1" {
		t.Skip("explicit read-only report opt-in")
	}
	endpoint := netip.MustParseAddrPort("127.0.0.1:50052")
	pids, err := ListCandidates()
	if err != nil {
		t.Fatal(err)
	}
	for _, pid := range pids {
		targets, err := discoverUpgradeTargets(endpoint, []int{pid}, true)
		if err != nil {
			t.Logf("UNRESOLVED pid=%d: %v", pid, err)
			continue
		}
		for _, target := range targets {
			t.Logf("TARGET (not blocker) pid=%d executable=%s endpoint=%s", target.Identity.PID, target.Identity.Executable, target.endpoint)
		}
	}
}
