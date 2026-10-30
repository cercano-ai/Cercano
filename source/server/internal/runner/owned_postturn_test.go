package runner

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/llm"
)

type postTurnSchedulingProbe struct{ ids []string }

func (p *postTurnSchedulingProbe) Schedule(id string)                     { p.ids = append(p.ids, id) }
func (*postTurnSchedulingProbe) CompactNow(context.Context, string) error { return nil }
func (*postTurnSchedulingProbe) IsCompacting(string) bool                 { return false }

func TestCore_PostTurnLegacyScheduling(t *testing.T) {
	recap, compaction := &postTurnSchedulingProbe{}, &postTurnSchedulingProbe{}
	deps := buildDeps(&spyProvider{})
	deps.Agent = agent.NewAgent(nil, nil,
		agent.WithRecapScheduler(recap), agent.WithCompactionScheduler(compaction))
	_, err := New(deps).RunTurn(context.Background(), Request{
		ConversationID: "legacy-turn", Input: "hello", WorkDir: t.TempDir(),
	}, noopSink{}, nil, func(llm.Message) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []*postTurnSchedulingProbe{recap, compaction} {
		if !reflect.DeepEqual(p.ids, []string{"legacy-turn"}) {
			t.Fatalf("legacy scheduling calls = %v, want one legacy-turn", p.ids)
		}
	}
}

// Exercise RunTurn itself: host worker bookkeeping cannot protect the ordinary
// in-process path if Core still calls the ID-only scheduling methods.
func TestCore_PostTurnOwnedScheduling(t *testing.T) {
	for _, rejectAt := range []string{"", "recap", "compaction"} {
		t.Run("reject_"+rejectAt, func(t *testing.T) {
			type capabilityKey struct{}
			capability := new(int)
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), capabilityKey{}, capability))
			defer cancel()
			denied := errors.New("ownership admission denied")
			var calls []string
			schedule := func(kind string) func(context.Context, string) error {
				return func(got context.Context, id string) error {
					if got.Value(capabilityKey{}) != capability || got.Done() != ctx.Done() {
						t.Error("post-turn scheduling lost the admitted capability or cancellation")
					}
					if id != "owned-turn" {
						t.Errorf("conversation = %q, want owned-turn", id)
					}
					calls = append(calls, kind)
					if rejectAt == kind {
						return denied
					}
					return nil
				}
			}
			deps := buildDeps(&spyProvider{})
			deps.Agent = agent.NewAgent(nil, nil, agent.WithOwnedScheduling(&agent.OwnedScheduling{
				Recap: schedule("recap"), Compaction: schedule("compaction"),
			}))
			_, err := New(deps).RunTurn(ctx, Request{
				ConversationID: "owned-turn", Input: "hello", WorkDir: t.TempDir(), Gen: 1,
			}, noopSink{}, nil, func(llm.Message) {})
			if rejectAt == "" && err != nil {
				t.Fatalf("RunTurn: %v", err)
			}
			if rejectAt != "" && !errors.Is(err, denied) {
				t.Fatalf("RunTurn error = %v, want ownership denial", err)
			}
			want := []string{"recap", "compaction"}
			if rejectAt == "recap" {
				want = want[:1]
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("scheduling calls = %v, want %v", calls, want)
			}
		})
	}
}

func TestCore_PostTurnWithoutPersistenceDoesNotSchedule(t *testing.T) {
	deps := buildDeps(&spyProvider{})
	unexpected := func(context.Context, string) error {
		t.Error("scheduled background work without persisted conversation")
		return nil
	}
	deps.Agent = agent.NewAgent(nil, nil, agent.WithOwnedScheduling(&agent.OwnedScheduling{
		Recap: unexpected, Compaction: unexpected,
	}))
	_, err := New(deps).RunTurn(context.Background(), Request{
		ConversationID: "not-persisted", Input: "hello", WorkDir: t.TempDir(),
	}, noopSink{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
}
