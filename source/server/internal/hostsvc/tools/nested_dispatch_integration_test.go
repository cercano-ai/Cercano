package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/capabilities/agentadapter"
	"cercano/source/server/internal/capabilities/builtins"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/taskrun"
)

type nestedFixtureProvider struct {
	historyProbeProvider
	depth, turns int
	release      <-chan struct{}
	useful       chan<- int
	leafFailure  bool
}

func (p *nestedFixtureProvider) StreamChat(ctx context.Context, r llm.ChatRequest) (llm.StreamReader, error) {
	p.turns++
	events := []llm.StreamEvent{{Type: llm.EventMessageStart}}
	if p.depth > 0 && p.turns == 1 {
		args := fmt.Sprintf(`{"task":"%d","tools":["Read","dispatch"]}`, p.depth-1)
		events = append(events, llm.StreamEvent{Type: llm.EventToolUseStart, ToolName: "dispatch", ToolUseID: "nested"}, llm.StreamEvent{Type: llm.EventToolUseInputDelta, TextDelta: args}, llm.StreamEvent{Type: llm.EventToolUseStop})
	} else {
		text := "leaf evidence"
		if p.depth == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-p.release:
			}
			if p.leafFailure {
				return nil, fmt.Errorf("leaf fixture failure")
			}
		} else if p.turns == 2 {
			// This useful parent model turn must happen while its child is blocked.
			p.useful <- p.depth
			text = "tentative parent synthesis"
		} else {
			data, _ := json.Marshal(r.Messages)
			if !strings.Contains(string(data), "Immediate child outcomes") {
				return nil, fmt.Errorf("parent continuation lost child evidence")
			}
			text = fmt.Sprintf("processed immediate child outcomes at depth %d", p.depth)
		}
		events = append(events, llm.StreamEvent{Type: llm.EventTextDelta, TextDelta: text})
	}
	events = append(events, llm.StreamEvent{Type: llm.EventMessageStop, StopReason: "end_turn"})
	return &sliceStream{events: events}, nil
}

// Tier 1: real capability adapter -> task manager -> dispatch tool loop ->
// recursive model continuation, with an isolated SQLite store and fake inference.
func TestNestedDispatchRecursiveLifecycle(t *testing.T) {
	for _, mode := range []string{"completed", "failed-child", "cancelled-parent"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			store, err := conversation.Open(filepath.Join(t.TempDir(), "nested.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			release := make(chan struct{})
			useful := make(chan int, 2)
			svc := New(nil, nil, nil, nil)
			installTestFailureLog(t, svc)
			manager, err := taskrun.New(store.(conversation.TaskStore), func(ctx context.Context, spec dispatch.Spec) (dispatch.Result, error) {
				var depth int
				if _, err := fmt.Sscanf(spec.Task, "%d", &depth); err != nil {
					return dispatch.Result{}, err
				}
				p := &nestedFixtureProvider{historyProbeProvider: historyProbeProvider{name: "anthropic"}, depth: depth, release: release, useful: useful, leafFailure: mode == "failed-child"}
				return svc.RunAgenticDispatch(ctx, spec, inference.Selection{Provider: p, IsCloud: true}, "fixture")
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
				defer stop()
				if err := manager.Close(stopCtx); err != nil {
					t.Error(err)
				}
			}()
			caps := capabilities.NewRegistry(capabilities.Services{Tasks: manager})
			builtins.Register(caps)
			svc.capRegistry = caps
			svc.SetRegistry(agentadapter.BuildAgentRegistry(caps, builtins.AgentAliases(), builtins.CapabilitySynonyms()))
			top, err := manager.Launch(ctx, dispatch.Spec{ConversationID: "root", Task: "2", Tools: []string{"Read", "dispatch"}, MaxIterations: 8})
			if err != nil {
				t.Fatal(err)
			}
			// Both ancestors must continue before the leaf returns. This catches an
			// eager join at the next model turn as well as synchronous delegation.
			for i := 0; i < 2; i++ {
				select {
				case <-useful:
				case <-ctx.Done():
					t.Fatal("parent did not continue useful work", ctx.Err())
				}
			}
			if rec, err := manager.Inspect(ctx, "root", top.ID); err != nil || rec.Terminal() {
				t.Fatalf("premature completion: %+v %v", rec, err)
			}
			if mode == "cancelled-parent" {
				if _, err := manager.Stop(ctx, "root", top.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				close(release)
			}
			rec, err := manager.Wait(ctx, "root", top.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "cancelled-parent" {
				if rec.Status != "stopped" {
					t.Fatalf("cancel status: %+v", rec)
				}
			} else if rec.Status != "completed" || !strings.Contains(string(rec.ResultJSON), "processed immediate child outcomes at depth 2") {
				t.Fatalf("unprocessed result: %+v", rec)
			}
			// Each generation's shutdown/result delivery is complete, not just the root.
			for {
				all, err := store.(conversation.TaskSubscriptionStore).TaskSnapshotPage(ctx, "root", "")
				if err != nil {
					t.Fatal(err)
				}
				done := len(all) == 3
				for _, task := range all {
					if !task.Terminal() {
						done = false
					}
				}
				if done {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal("descendant cancellation deadlocked")
				case <-time.After(10 * time.Millisecond):
				}
			}
		})
	}
}
