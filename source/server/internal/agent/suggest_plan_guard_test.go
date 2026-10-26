package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
)

type profileControlTool struct {
	stubTool
	run func()
}

func (t profileControlTool) Execute(context.Context, json.RawMessage) (*agenttools.Result, error) {
	t.run()
	return &agenttools.Result{Text: "ok"}, nil
}

type profileTransitionProvider struct {
	*mockProvider
	transition func()
}

func (p *profileTransitionProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (llm.StreamReader, error) {
	if p.calls == 0 && p.transition != nil {
		p.transition()
	}
	return p.mockProvider.StreamChat(ctx, req)
}

func TestSuggestPlan_LiveEntryGuard(t *testing.T) {
	for _, mode := range []PermissionMode{ModeStrict, ModePermissive, ModeBypass} {
		for _, scenario := range []string{"first", "already", "same_batch", "consecutive", "external", "local_fallback", "plan_exit", "request_plan_approval"} {
			t.Run(string(mode)+"/"+scenario, func(t *testing.T) {
				active := Profile{}
				initial := Profile{}
				calls := []string{"suggest_plan"}
				wantEntries, wantBlocked := 1, 0
				if scenario == "already" {
					active = PlanProfile()
					initial = active
					wantEntries = 0
					wantBlocked = 1
				}
				if scenario == "external" {
					wantEntries = 0
					wantBlocked = 1
				}
				if scenario == "same_batch" || scenario == "consecutive" || scenario == "local_fallback" {
					calls = append(calls, "suggest_plan")
					wantBlocked = 1
				}
				if scenario == "plan_exit" || scenario == "request_plan_approval" {
					calls = append(calls, scenario, "suggest_plan")
					wantEntries = 2
				}
				var scripts [][]llm.Block
				for i, name := range calls {
					block := llm.Block{Type: llm.BlockToolUse, ToolUseID: fmt.Sprint(i), ToolName: name, ToolInput: json.RawMessage(`{}`)}
					if scenario == "same_batch" || scenario == "local_fallback" {
						if len(scripts) == 0 {
							scripts = append(scripts, nil)
						}
						scripts[0] = append(scripts[0], block)
					} else {
						scripts = append(scripts, []llm.Block{block})
					}
				}
				scripts = append(scripts, []llm.Block{{Type: llm.BlockText, Text: "done"}})
				p := &profileTransitionProvider{mockProvider: &mockProvider{scripts: scripts, caps: inference.Capabilities{SupportsTools: true}}}
				if scenario == "external" {
					p.transition = func() { active = PlanProfile() }
				}
				entries, exits, prompts, permissionEvents := 0, 0, 0, 0
				reg := agenttools.NewRegistry()
				reg.Register(profileControlTool{stubTool: stubTool{name: "suggest_plan", perm: agenttools.PermX}, run: func() { entries++; active = PlanProfile() }})
				for _, name := range []string{"plan_exit", "request_plan_approval"} {
					reg.Register(profileControlTool{stubTool: stubTool{name: name, perm: agenttools.PermX}, run: func() { exits++; active = Profile{} }})
				}
				perms, _ := LoadPermissionStore(t.TempDir() + "/permissions.yaml")
				if err := perms.SetMode(mode); err != nil {
					t.Fatal(err)
				}
				var live func(context.Context) (Profile, error)
				if scenario != "local_fallback" {
					live = func(context.Context) (Profile, error) { return active, nil }
				}
				result, err := RunToolLoop(t.Context(), ToolLoopInput{Provider: p, Registry: reg, Permissions: perms, Profile: initial, CurrentProfile: live, UserInput: "plan", PermissionRequester: func(context.Context, string, string, json.RawMessage, llm.Permission, bool) (bool, error) {
					prompts++
					return true, nil
				}, EventSink: func(e LoopEvent) {
					if e.Kind == LoopPermissionRequired {
						permissionEvents++
					}
				}})
				if err != nil {
					t.Fatal(err)
				}
				if entries != wantEntries {
					t.Fatalf("entries=%d want %d", entries, wantEntries)
				}
				wantPrompts := entries + exits
				if mode == ModeBypass {
					wantPrompts = entries
					if scenario == "request_plan_approval" {
						wantPrompts += exits
					}
				}
				if prompts != wantPrompts || permissionEvents != wantPrompts {
					t.Fatalf("prompts=%d events=%d want %d", prompts, permissionEvents, wantPrompts)
				}
				blocked := 0
				for _, m := range result.History {
					for _, b := range m.Blocks {
						if b.Type == llm.BlockToolResult && b.IsError && strings.Contains(b.Content, "already in planning mode") {
							blocked++
						}
					}
				}
				if blocked != wantBlocked {
					t.Fatalf("self-aware denials=%d want %d", blocked, wantBlocked)
				}
				if scenario == "consecutive" || scenario == "first" {
					for _, tool := range p.reqs[1].Tools {
						if tool.Name == "suggest_plan" {
							t.Fatal("entry did not refresh catalog")
						}
					}
				}
			})
		}
	}
}

func TestSuggestPlan_QueuedExecutionFence(t *testing.T) {
	for _, mode := range []PermissionMode{ModeStrict, ModePermissive, ModeBypass} {
		for _, exit := range []string{"", "plan_exit", "request_plan_approval"} {
			t.Run(string(mode)+"/"+exit, func(t *testing.T) {
				names := []string{"suggest_plan", "Bash", "ordinary_write"}
				if exit != "" {
					names = append(names, exit, "Bash")
				}
				var batch []llm.Block
				for i, name := range names {
					batch = append(batch, llm.Block{Type: llm.BlockToolUse, ToolUseID: fmt.Sprint(i), ToolName: name, ToolInput: json.RawMessage(`{}`)})
				}
				p := &mockProvider{scripts: [][]llm.Block{batch, {{Type: llm.BlockText, Text: "done"}}}, caps: inference.Capabilities{SupportsTools: true}}
				reg := agenttools.NewRegistry()
				runs := map[string]int{}
				for _, name := range []string{"suggest_plan", "Bash", "ordinary_write", "plan_exit", "request_plan_approval"} {
					tier := agenttools.PermX
					if name == "ordinary_write" {
						tier = agenttools.PermW
					}
					reg.Register(profileControlTool{stubTool: stubTool{name: name, perm: tier}, run: func() { runs[name]++ }})
				}
				perms, _ := LoadPermissionStore(t.TempDir() + "/permissions.yaml")
				if err := perms.SetMode(mode); err != nil {
					t.Fatal(err)
				}
				forbiddenPrompts, forbiddenEvents, forbiddenWatchdog := 0, 0, 0
				result, err := RunToolLoop(t.Context(), ToolLoopInput{Provider: p, Registry: reg, Permissions: perms, UserInput: "plan",
					WatchdogGate: func(_ context.Context, name string, _ json.RawMessage, _ []llm.Message) WatchdogDecision {
						if (name == "Bash" || name == "ordinary_write") && runs["plan_exit"] == 0 && runs["request_plan_approval"] == 0 {
							forbiddenWatchdog++
						}
						return WatchdogDecision{}
					},
					PermissionRequester: func(_ context.Context, id, _ string, _ json.RawMessage, _ llm.Permission, _ bool) (bool, error) {
						if id == "1" || id == "2" {
							forbiddenPrompts++
						}
						return true, nil
					},
					EventSink: func(e LoopEvent) {
						if (e.ToolUseID == "1" || e.ToolUseID == "2") && (e.Kind == LoopPermissionRequired || e.Kind == LoopToolExecStart) {
							forbiddenEvents++
						}
					},
				})
				if forbiddenWatchdog != 0 {
					t.Fatalf("forbidden watchdog calls=%d", forbiddenWatchdog)
				}
				if err != nil {
					t.Fatal(err)
				}
				wantBash := 0
				if exit != "" {
					wantBash = 1
					if runs[exit] != 1 {
						t.Fatalf("exit runs=%v", runs)
					}
				}
				if runs["suggest_plan"] != 1 || runs["Bash"] != wantBash || runs["ordinary_write"] != 0 || forbiddenPrompts != 0 || forbiddenEvents != 0 {
					t.Fatalf("runs=%v forbidden prompts=%d events=%d", runs, forbiddenPrompts, forbiddenEvents)
				}
				blocked := 0
				for _, m := range result.History {
					for _, b := range m.Blocks {
						if b.Type == llm.BlockToolResult && (b.ToolUseRef == "1" || b.ToolUseRef == "2") && b.IsError && strings.Contains(b.Content, "plan") {
							blocked++
						}
					}
				}
				if blocked != 2 {
					t.Fatalf("denials=%d want 2", blocked)
				}
			})
		}
	}
}
