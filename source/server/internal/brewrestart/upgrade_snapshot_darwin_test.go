//go:build darwin && cgo

package brewrestart

import (
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Explicit opt-in, read-only observations. Never invokes shutdown or signals;
// these racy snapshots cannot authorize signalling or migration.
func TestReadOnlyUpgradeChildSnapshot(t *testing.T) {
	raw := os.Getenv("CERCANO_SNAPSHOT_PIDS")
	if raw == "" {
		t.Skip("requires explicit read-only PID inventory")
	}
	pids := strings.Split(raw, ",")
	if len(pids) > 32 {
		t.Fatal("PID inventory exceeds 32")
	}
	for _, text := range pids {
		pid, err := strconv.Atoi(text)
		if err != nil {
			t.Fatal("invalid PID")
		}
		host, err := Inspect(pid)
		if err != nil {
			t.Logf("racy observation pid=%d identity unavailable", pid)
			continue
		}
		role, err := processRole(host)
		if err != nil {
			t.Logf("racy observation pid=%d role unavailable", pid)
			continue
		}
		t.Logf("racy observation pid=%d role=%s (not signal authority)", pid, safeUpgradeRole(role))
		if role != "agent" && role != "development-host" {
			continue
		}
		endpoint := netip.MustParseAddrPort("127.0.0.1:50052")
		owns, listenerErr := HoldsListener(host, endpoint)
		t.Logf("host pid=%d target_listener=%t listener_inspection_ok=%t", pid, owns, listenerErr == nil)
		children, err := upgradeChildren(pid)
		if err != nil {
			t.Fatal(err)
		}
		sort.Ints(children)
		if len(children) > 64 {
			t.Logf("children omitted=%d", len(children)-64)
			children = children[:64]
		}
		for _, childPID := range children {
			id, err := Inspect(childPID)
			if err != nil {
				t.Logf("child pid=%d identity unavailable", childPID)
				continue
			}
			r, roleErr := processRole(id)
			shared, sharedErr := verifiedModelHost(id)
			category, reason := upgradeDirectChildCategory(host, id, r)
			if roleErr != nil {
				category, reason = "unresolved", "role unavailable"
			} else if sharedErr != nil {
				category, reason = "unresolved", "model-host classification unavailable"
			} else if shared {
				category, reason = "shared-model-host", "excluded"
			}

			rAgain, rErr := processRole(id)
			now, e := Inspect(childPID)
			stable := rErr == nil && rAgain == r && e == nil && now.SameProcess(id) && now.ParentPID == pid && id.ParentPID == pid && id.UID == host.UID && (id.StartSeconds > host.StartSeconds || (id.StartSeconds == host.StartSeconds && id.StartMicroseconds >= host.StartMicroseconds))
			t.Logf("child pid=%d role=%s category=%s reason=%s revalidated=%t", childPID, safeUpgradeRole(r), category, reason, stable)
		}
		roleAgain, roleErr := processRole(host)
		ownsAgain, listenErr := HoldsListener(host, endpoint)
		now, err := Inspect(pid)
		t.Logf("host pid=%d revalidated=%t; entire observation remains racy", pid, err == nil && now.SameProcess(host) && roleErr == nil && roleAgain == role && listenErr == nil && listenerErr == nil && owns == ownsAgain)
	}
}
