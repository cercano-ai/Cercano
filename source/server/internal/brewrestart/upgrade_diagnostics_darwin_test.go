//go:build darwin && cgo

package brewrestart

import (
	"strings"
	"testing"
)

func TestUpgradeChildDiagnosticsBoundedAndRedacted(t *testing.T) {
	var d childResolutionDiagnostics
	for pid := 100; pid > 0; pid-- {
		d.add(pid, "role="+safeUpgradeRole("secret-token\nforged log")+" reason=unsupported role")
	}
	text := d.String()
	if len(d.entries) != 8 || len(text) > 1000 || !strings.Contains(text, "unresolved=100 omitted=92") || strings.Contains(text, "secret") || strings.Contains(text, "\n") {
		t.Fatalf("unbounded or unsafe diagnostic: %q", text)
	}
	for i, e := range d.entries {
		if e.pid != i+1 {
			t.Fatalf("unstable PID ordering: %+v", d.entries)
		}
	}
	for role, want := range map[string]string{"worker": "worker", "model-host": "model-host", "agent": "agent", "": "missing", "--token=secret": "unrecognized"} {
		if got := safeUpgradeRole(role); got != want {
			t.Errorf("role classification got %q want %q", got, want)
		}
	}
}
