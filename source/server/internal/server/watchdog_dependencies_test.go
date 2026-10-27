package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/watchdog"
	"cercano/source/server/pkg/config"
)

func TestWatchdogInvocationDependencies(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	var servers [2]*Server
	calls := [2]int{}
	opens := [2]int{}
	for i := range servers {
		path := filepath.Join(t.TempDir(), "audit.db")
		s := &Server{}
		s.SetWatchdogDependencies(WatchdogDependencies{
			OpenAudit: func(got string) (watchdog.AuditLogger, error) {
				opens[i]++
				if got != path {
					t.Fatalf("path = %q", got)
				}
				return nil, errors.New("fixture audit unavailable")
			},
			OneShot: func(context.Context, string) (string, error) {
				calls[i]++
				return "VIOLATION: yes\nCHALLENGE: use plain english", nil
			},
		})
		wc := config.WatchdogConfig{Checks: []string{"plain-english"}, Audit: config.WatchdogAuditConfig{Enabled: true, Path: path}}
		if s.buildWatchdogFrom(wc, config.ModelsConfig{}) != nil || opens[i] != 0 {
			t.Fatal("disabled watchdog touched dependency")
		}
		wc.Enabled = true
		s.watchdog = s.buildWatchdogFrom(wc, config.ModelsConfig{})
		servers[i] = s
	}
	for _, i := range []int{0, 1, 1} {
		d := servers[i].watchdog.Gate(context.Background(), "fixture", watchdog.Action{Kind: "turn_end", Text: "Here is the stack trace: 0x00deadbeef null ptr"})
		if d.Action == "allow" {
			t.Fatal("audit failure disabled supervision")
		}
	}
	if calls != [2]int{1, 2} || opens != [2]int{1, 1} {
		t.Fatalf("calls=%v opens=%v", calls, opens)
	}
}
