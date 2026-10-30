package builtins

import (
	"cercano/source/server/internal/agenttools"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/pkg/config"
)

func TestDispatch_Meta(t *testing.T) {
	c := Dispatch()
	if c.Name() != "dispatch" {
		t.Errorf("Name() = %q, want %q", c.Name(), "dispatch")
	}
	if c.Tier() != capabilities.TierR {
		t.Errorf("Tier() = %q, want TierR", c.Tier())
	}
	if !c.Surfaces().Has(capabilities.SurfaceAgent) {
		t.Error("missing SurfaceAgent")
	}
	if !c.Surfaces().Has(capabilities.SurfaceMCP) {
		t.Error("missing SurfaceMCP")
	}
}

type launchProbe struct {
	capabilities.Tasks
	launch func(context.Context, dispatch.Spec) (conversation.TaskRecord, error)
}

func (p launchProbe) Launch(ctx context.Context, spec dispatch.Spec) (conversation.TaskRecord, error) {
	return p.launch(ctx, spec)
}

func TestDispatchLaunchContract(t *testing.T) {
	for _, tier := range []string{"", "light", "standard", "deep"} {
		t.Run(tier, func(t *testing.T) {
			var captured dispatch.Spec
			svc := capabilities.Services{Tasks: launchProbe{launch: func(_ context.Context, spec dispatch.Spec) (conversation.TaskRecord, error) {
				captured = spec
				return conversation.TaskRecord{ID: "task-1", ChildID: "child-1", Status: "accepted"}, nil
			}}, Dispatch: func(context.Context, dispatch.Spec) (dispatch.Result, error) {
				t.Fatal("synchronous hook called")
				return dispatch.Result{}, nil
			}}
			args, _ := json.Marshal(map[string]any{"task": "work", "tools": []string{"Read"}, "tier": tier, "cwd": "/explicit"})
			result, err := Dispatch().Execute(t.Context(), &capabilities.Call{Args: args, ConversationID: "owner", WorkDir: "/ambient", Svc: svc, Emit: func(string) { t.Error("main-turn callback retained") }})
			if err != nil {
				t.Fatal(err)
			}
			if captured.Mode != dispatch.Agentic || captured.Role != dispatch.RoleCoproc || captured.RoutingTask != config.TaskDispatch || captured.Tier != tierForDispatch(tier) || captured.WorkDir != "/explicit" || captured.ConversationID != "owner" || captured.Task != "work" || len(captured.Tools) != 1 || captured.Emit != nil {
				t.Fatalf("spec: %+v", captured)
			}
			var handle map[string]string
			if err := json.Unmarshal([]byte(result.Text), &handle); err != nil {
				t.Fatal(err)
			}
			if handle["task_id"] != "task-1" || handle["child_id"] != "child-1" || handle["status"] != "accepted" {
				t.Fatalf("handle: %v", handle)
			}
		})
	}
}
func TestDispatchLaunchFailsClosed(t *testing.T) {
	for _, tc := range []struct{ parent, args string }{
		{"", `{"task":"work","conversation_id":"forged"}`},
		{"owner", `{"task":"work","conversation_id":"foreign"}`},
		{"owner", `{"task":""}`},
		{"owner", `{"task":"work","class":"invalid"}`},
	} {
		svc := capabilities.Services{Tasks: launchProbe{launch: func(context.Context, dispatch.Spec) (conversation.TaskRecord, error) {
			t.Fatal("invalid launch reached manager")
			return conversation.TaskRecord{}, nil
		}}}
		if _, err := Dispatch().Execute(t.Context(), &capabilities.Call{ConversationID: tc.parent, Args: json.RawMessage(tc.args), Svc: svc}); err == nil {
			t.Fatalf("accepted %s", tc.args)
		}
	}
	if _, err := Dispatch().Execute(t.Context(), &capabilities.Call{ConversationID: "owner", Args: json.RawMessage(`{"task":"work"}`)}); err == nil {
		t.Fatal("missing service accepted")
	}
}

// TestDispatchTierFor pins the dynamic tier: read-only grants stay R (silent
// in every mode); write-capable, unknown, and prefix-mangled-unknown
// grants escalate the dispatch call to X so a human confirms the sub-agent's
// toolset once. Display aliases and synonyms resolve like the live registry.
func TestDispatchTierFor(t *testing.T) {
	d := dispatchCap{}
	cases := []struct {
		args string
		want capabilities.Tier
	}{
		{`{"task":"x"}`, capabilities.TierR},                              // default read-only grant
		{`{"task":"x","tools":["Read","Grep","LS"]}`, capabilities.TierR}, // all R
		{`{"task":"x","tools":["get_protocol"]}`, capabilities.TierR},     // canonical R name
		{`{"task":"x","tools":["Read","Edit"]}`, capabilities.TierX},      // W in grant
		{`{"task":"x","tools":["RunCommand"]}`, capabilities.TierX},
		{`{"task":"x","tools":["Bash"]}`, capabilities.TierX},          // W via alias
		{`{"task":"x","tools":["git_push"]}`, capabilities.TierX},      // X in grant
		{`{"task":"x","tools":["mcp__oc__Read"]}`, capabilities.TierR}, // prefix-stripped R
		{`{"task":"x","tools":["mystery_tool"]}`, capabilities.TierX},  // unknown → conservative
		{`{"task":"x","tools":["workflow"]}`, capabilities.TierX},      // synonym of dispatch (W)
	}
	for i, c := range cases {
		if got := d.TierFor(json.RawMessage(c.args)); got != c.want {
			t.Errorf("case %d %s: TierFor = %q, want %q", i, c.args, got, c.want)
		}
	}
}

func TestRunCommandGrantAliasesHaveWriteTier(t *testing.T) {
	for _, name := range []string{"Bash", "RunCommand"} {
		if agentGrantTiers()[name] != capabilities.TierW {
			t.Fatalf("missing write tier for %s", name)
		}
	}
}

func TestDispatchSchemaDescription(t *testing.T) {
	c := Dispatch()
	schema := c.Schema()

	// Verify the schema contains the deliberate override guidance
	if !strings.Contains(string(schema), "Omit tier for normal dispatches to honor the selected class's saved quality") {
		t.Error("schema should instruct to omit tier for normal dispatches to honor saved quality")
	}

	// Verify the schema contains the deliberate override guidance
	if !strings.Contains(string(schema), "Supply tier only for a deliberate quality override") {
		t.Error("schema should instruct to supply tier only for deliberate quality override")
	}

	// Verify the schema contains the override behavior
	if !strings.Contains(string(schema), "explicit values override the saved setting") {
		t.Error("schema should document that explicit values override the saved setting")
	}

	// Verify the tier options are still documented
	if !strings.Contains(string(schema), "Explicit light selects Economy, standard selects Standard, and deep selects Premium") {
		t.Error("schema should document tier selection behavior")
	}
}

func TestDispatchLaunchEmitsChildLinkOnlyAfterAcceptance(t *testing.T) {
	for _, fail := range []bool{false, true} {
		var events []agenttools.ProgressEvent
		accepted := false
		svc := capabilities.Services{Tasks: launchProbe{launch: func(_ context.Context, spec dispatch.Spec) (conversation.TaskRecord, error) {
			if spec.Emit != nil {
				t.Fatal("launch retained foreground callback")
			}
			if fail {
				return conversation.TaskRecord{}, errors.New("launch failed")
			}
			accepted = true
			return conversation.TaskRecord{ID: "task", ChildID: "child", Status: "accepted"}, nil
		}}}
		_, err := Dispatch().Execute(t.Context(), &capabilities.Call{Args: json.RawMessage(`{"task":"work"}`), ConversationID: "owner", Svc: svc, EmitProgress: func(e agenttools.ProgressEvent) {
			if !accepted {
				t.Error("event before acceptance")
			}
			events = append(events, e)
		}})
		if fail {
			if err == nil || len(events) != 0 {
				t.Fatalf("failed launch emitted link: %v %v", err, events)
			}
			continue
		}
		if err != nil || len(events) != 1 {
			t.Fatalf("accepted launch needs one link event: %v %v", err, events)
		}
		e := events[0]
		if e.Kind != "started" || e.SubAgentID != "child" || e.SubAgentParentID != "owner" {
			t.Fatalf("wrong link event: %+v", e)
		}
	}
}
