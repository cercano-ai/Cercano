//go:build darwin && cgo

package brewrestart

import (
	"strings"
	"syscall"
	"testing"
)

func TestUpgradeGraceLifetime(t *testing.T) {
	id := Identity{PID: 42, StartSeconds: 10, StartMicroseconds: 20, Executable: "/original"}
	for _, tc := range []struct {
		name       string
		now        Identity
		inspectErr error
		refusal    bool
		remaining  int
	}{
		{"unchanged", id, nil, false, 1},
		{"exec", Identity{PID: 42, StartSeconds: 10, StartMicroseconds: 20, Executable: "/changed"}, nil, true, 0},
		{"reused", Identity{PID: 42, StartSeconds: 10, StartMicroseconds: 21, Executable: "/changed"}, nil, false, 0},
		{"gone", Identity{}, syscall.ESRCH, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			targets := map[int]Identity{id.PID: id}
			err := observeUpgradeExit(targets, id, tc.now, tc.inspectErr)
			if tc.refusal && (err == nil || !strings.Contains(err.Error(), "changed identity without exiting")) {
				t.Fatalf("expected actionable exec refusal, got %v", err)
			}
			if !tc.refusal && err != nil {
				t.Fatal(err)
			}
			// Both escalation and deferred cleanup signal only this set: a changed
			// identity must never remain eligible for either signal path.
			if len(targets) != tc.remaining {
				t.Fatalf("signal targets = %v", targets)
			}
		})
	}
}
