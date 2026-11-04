package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
)

func TestChildResultsJoinContinuesModelBeforeFinalization(t *testing.T) {
	p := &mockProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{
		{{Type: llm.BlockText, Text: "parent useful work and tentative conclusion"}},
		{{Type: llm.BlockText, Text: "processed child result"}},
	}}
	joins := 0
	result, err := RunToolLoop(context.Background(), ToolLoopInput{
		Provider: p, Model: "mock", Registry: agenttools.NewRegistry(), UserInput: "work", MaxIterations: 5,
		ChildResults: func(ctx context.Context, join bool) (string, error) {
			if !join {
				return "", nil
			}
			joins++
			if p.calls == 0 {
				t.Fatal("joined before useful parent work")
			}
			if joins == 1 {
				return "child finished with evidence", nil
			}
			return "", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.calls != 2 || joins != 2 || result.FinalText != "processed child result" {
		t.Fatalf("calls=%d joins=%d result=%+v", p.calls, joins, result)
	}
	found := false
	for _, m := range p.reqs[1].Messages {
		for _, b := range m.Blocks {
			if strings.Contains(b.Text, "child finished with evidence") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("parent continuation never received child evidence")
	}
}

func TestChildResultsEarlyDeliveryAndCancellation(t *testing.T) {
	p := &mockProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{{{Type: llm.BlockText, Text: "done"}}}}
	_, err := RunToolLoop(context.Background(), ToolLoopInput{Provider: p, Model: "mock", Registry: agenttools.NewRegistry(), UserInput: "work", MaxIterations: 3,
		ChildResults: func(ctx context.Context, join bool) (string, error) {
			if join {
				return "", context.Canceled
			}
			return "already completed child", nil
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	found := false
	for _, m := range p.reqs[0].Messages {
		for _, b := range m.Blocks {
			if b.Text == "already completed child" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("available evidence delayed until finalization")
	}
}

// A forced-confirm call can follow a dispatch even with an explicit tool grant.
// Every successful permission/control stop must still process its child outcome.
func TestChildResultsPermissionEarlyCompletion(t *testing.T) {
	for _, kind := range []string{"missing requester", "denied", "control redirect", "control failure"} {
		t.Run(kind, func(t *testing.T) {
			tool := &childStopTool{name: "plan_experiment", fail: kind == "control failure"}
			if strings.HasPrefix(kind, "control") {
				tool.name = "session_model"
			}
			registry := agenttools.NewRegistry()
			registry.Register(tool)
			p := &mockProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{
				{{Type: llm.BlockToolUse, ToolUseID: "stop", ToolName: tool.name, ToolInput: json.RawMessage(`{}`)}, {Type: llm.BlockToolUse, ToolUseID: "skipped", ToolName: tool.name, ToolInput: json.RawMessage(`{}`)}},
				{{Type: llm.BlockText, Text: "processed child evidence"}},
			}}
			in := ToolLoopInput{Provider: p, Model: "mock", Registry: registry, UserInput: "work", MaxIterations: 3}
			if kind == "missing requester" {
				in.PreauthorizedTools = []string{tool.name}
			}
			if kind != "missing requester" {
				in.PermissionRequester = func(context.Context, string, string, json.RawMessage, llm.Permission, bool) (bool, error) {
					if kind == "control redirect" {
						return false, &FollowUpDenial{Message: "stop control"}
					}
					return kind == "control failure", nil
				}
			}
			joins := 0
			in.ChildResults = func(_ context.Context, join bool) (string, error) {
				if !join {
					return "", nil
				}
				joins++
				if joins == 1 {
					return "child evidence", nil
				}
				return "", nil
			}
			result, err := RunToolLoop(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			if p.calls != 2 || result.FinalText != "processed child evidence" || joins != 2 {
				t.Fatalf("premature completion: calls=%d joins=%d final=%q", p.calls, joins, result.FinalText)
			}
			if (kind == "control failure" && tool.calls != 1) || (kind != "control failure" && tool.calls != 0) {
				t.Fatalf("denied tool executed %d times", tool.calls)
			}
			skipped := false
			for _, m := range p.reqs[1].Messages {
				for _, b := range m.Blocks {
					if b.ToolUseRef == "skipped" && b.IsError {
						skipped = true
					}
				}
			}
			if !skipped {
				t.Fatal("unexecuted queued call missing error result")
			}
			if len(p.reqs[1].Tools) != 0 {
				t.Fatal("permission stop reopened tool execution")
			}
			found := false
			for _, m := range p.reqs[1].Messages {
				for _, b := range m.Blocks {
					if b.Text == "child evidence" {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("continuation missing child evidence")
			}
		})
	}
}

type childStopTool struct {
	name  string
	fail  bool
	calls int
}

func (x *childStopTool) Name() string                    { return x.name }
func (*childStopTool) Description() string               { return "test stop" }
func (*childStopTool) Permission() agenttools.Permission { return agenttools.PermX }
func (*childStopTool) Schema() json.RawMessage           { return json.RawMessage(`{"type":"object"}`) }
func (x *childStopTool) Execute(context.Context, json.RawMessage) (*agenttools.Result, error) {
	x.calls++
	if x.fail {
		return nil, errors.New("control failed")
	}
	return agenttools.NewTextResult("unexpected execution"), nil
}

func TestChildResultsPermissionStopWithoutOutcomes(t *testing.T) {
	for _, mode := range []string{"main chat", "empty join", "cancelled join"} {
		t.Run(mode, func(t *testing.T) {
			tool := &childStopTool{name: "plan_experiment"}
			reg := agenttools.NewRegistry()
			reg.Register(tool)
			p := &mockProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{
				{{Type: llm.BlockToolUse, ToolUseID: "denied", ToolName: tool.name, ToolInput: json.RawMessage(`{}`)}},
			}}
			in := ToolLoopInput{Provider: p, Model: "mock", Registry: reg, UserInput: "work", MaxIterations: -1}
			if mode != "main chat" {
				in.ChildResults = func(_ context.Context, join bool) (string, error) {
					if join && mode == "cancelled join" {
						return "", context.Canceled
					}
					return "", nil
				}
			}
			_, err := RunToolLoop(context.Background(), in)
			if mode == "cancelled join" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if p.calls != 1 || tool.calls != 0 {
				t.Fatalf("calls=%d executed=%d", p.calls, tool.calls)
			}
		})
	}
}
