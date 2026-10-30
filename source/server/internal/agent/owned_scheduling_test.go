package agent

import (
	"context"
	"testing"
)

type legacySchedulingProbe struct{ ids []string }

func (p *legacySchedulingProbe) Schedule(id string)                     { p.ids = append(p.ids, id) }
func (*legacySchedulingProbe) CompactNow(context.Context, string) error { return nil }
func (*legacySchedulingProbe) IsCompacting(string) bool                 { return false }

func TestLegacyContextSchedulingPreservesBehavior(t *testing.T) {
	r, c := &legacySchedulingProbe{}, &legacySchedulingProbe{}
	a := NewAgent(nil, nil, WithRecapScheduler(r), WithCompactionScheduler(c))
	if a.HasOwnedScheduling() {
		t.Fatal("legacy acquired owned scheduling")
	}
	if err := a.SchedulePersistedAssistant(context.Background(), "not-scheduled"); err != nil {
		t.Fatal(err)
	}
	if len(r.ids)+len(c.ids) != 0 {
		t.Fatal("legacy persistence gained scheduling")
	}
	if err := a.ScheduleRecapContext(context.Background(), "legacy"); err != nil {
		t.Fatal(err)
	}
	if err := a.ScheduleCompactionContext(context.Background(), "legacy"); err != nil {
		t.Fatal(err)
	}
	if len(r.ids) != 1 || len(c.ids) != 1 || r.ids[0] != "legacy" || c.ids[0] != "legacy" {
		t.Fatalf("legacy schedule changed: %v %v", r.ids, c.ids)
	}
}

// Foreground persistence must pass the incoming context unchanged; legacy
// foreground persistence still schedules exactly once per producer.
func TestOwnedForegroundSchedulingContext(t *testing.T) {
	for _, owned := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "owned"}[owned], func(t *testing.T) {
			a, store := newTestAgentWithPersistent(t)
			r, c := &legacySchedulingProbe{}, &legacySchedulingProbe{}
			WithRecapScheduler(r)(a)
			WithCompactionScheduler(c)(a)
			type authorityKey struct{}
			ctx := context.WithValue(context.Background(), authorityKey{}, "exact capability")
			calls := 0
			if owned {
				schedule := func(got context.Context, id string) error {
					if got != ctx || id != "foreground" {
						t.Error("lost incoming authority")
					}
					turns, err := store.GetTurns(got, id)
					if err != nil || len(turns) != 2 {
						t.Errorf("scheduled before successful persistence: %v %d", err, len(turns))
					}
					calls++
					return nil
				}
				WithOwnedScheduling(&OwnedScheduling{Recap: schedule, Compaction: schedule})(a)
			}
			a.storeConversationTurn(ctx, "foreground", "user", "user", &Response{Output: "assistant"})
			if owned {
				if calls != 2 || len(r.ids)+len(c.ids) != 0 {
					t.Fatal("owned scheduling fell through to legacy")
				}
			} else if len(r.ids) != 1 || len(c.ids) != 1 {
				t.Fatal("legacy foreground scheduling changed")
			}
		})
	}
}
