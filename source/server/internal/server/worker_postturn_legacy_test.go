package server

import (
	"context"
	"testing"

	"cercano/source/server/internal/agent"
	runnersvc "cercano/source/server/internal/runner"
)

// Both producer interfaces intentionally share this fixture. The ordinary
// startup must keep its ID-only producer behavior when no ownership bridge is
// attached; ownership-enabled agents must never fall back to those producers.
type postTurnSchedulingProbe struct {
	ids []string
}

func (p *postTurnSchedulingProbe) Schedule(id string)                     { p.ids = append(p.ids, id) }
func (*postTurnSchedulingProbe) CompactNow(context.Context, string) error { return nil }
func (*postTurnSchedulingProbe) IsCompacting(string) bool                 { return false }

func TestWorkerPostTurn_LegacyScheduling(t *testing.T) {
	srv, _ := newServerWithStore(t)
	recap, compaction := &postTurnSchedulingProbe{}, &postTurnSchedulingProbe{}
	agent.WithRecapScheduler(recap)(srv.agent)
	agent.WithCompactionScheduler(compaction)(srv.agent)
	if err := srv.workerPostTurn(context.Background(), "legacy-post-turn", runnersvc.Result{}); err != nil {
		t.Fatal(err)
	}
	for name, probe := range map[string]*postTurnSchedulingProbe{"recap": recap, "compaction": compaction} {
		if len(probe.ids) != 1 || probe.ids[0] != "legacy-post-turn" {
			t.Fatalf("%s scheduled %v, want legacy-post-turn once", name, probe.ids)
		}
	}
}
