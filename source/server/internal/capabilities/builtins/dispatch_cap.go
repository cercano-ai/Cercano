package builtins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/pkg/config"
)

type dispatchCap struct{}

// Dispatch constructs the dispatch capability.
func Dispatch() capabilities.Capability { return dispatchCap{} }

func (dispatchCap) Name() string            { return "dispatch" }
func (dispatchCap) Tier() capabilities.Tier { return capabilities.TierR }
func (dispatchCap) Surfaces() capabilities.Surface {
	return capabilities.SurfaceAgent | capabilities.SurfaceMCP
}
func (dispatchCap) Description() string {
	return "Launch an independent background sub-agent and return its durable task_id immediately. The task survives steering, main-turn cancellation, and client disconnect. Use task_list, task_inspect, task_wait, or task_stop to manage this chat's tasks; waiting does not cancel execution. Results, route metadata, tool evidence, and errors remain available through inspection. Tool names must be plain registered names (Read, Edit, RunCommand), not host-prefixed aliases. Write-capable grants require approval. Class selects saved routing; omit tier unless deliberately overriding quality. cwd/path selects the working directory. Requires host-bound chat identity; conversation_id arguments never establish ownership."
}

func (dispatchCap) Schema() capabilities.Schema {
	classes := []config.Task{}
	for _, d := range config.TaskDefinitions() {
		classes = append(classes, d.Task)
	}
	keys, _ := json.Marshal(classes)
	return capabilities.Schema(`{
		"type": "object",
		"required": ["task"],
		"properties": {
			"class": {"type":"string", "enum":` + string(keys) + `, "description":"Choose a class by work: reconnaissance for narrow read-only tracing/extraction; mechanical_development for specified edits; investigation for diagnosis; implementation for substantial coding. Dedicated Review, Research and Git land use review/research/git_land. Omission uses Default dispatch."},
 "task":            {"type": "string", "description": "Open-ended instruction for the sub-agent tool loop."},
			"tools":           {"type": "array", "items": {"type": "string"}, "description": "Tool or capability names to grant, using the plain registered names (e.g. \"Read\", \"Glob\", \"Grep\", \"RunCommand\") — no host or MCP prefix. Omit to default to read-only tools."},
			"tier":            {"type": "string", "enum": ["light", "standard", "deep"], "description": "Omit tier for normal dispatches to honor the selected class's saved quality. Supply tier only for a deliberate quality override; explicit values override the saved setting. An optional class selects the saved task routing; omitted class uses Default dispatch. Explicit light selects Economy, standard selects Standard, and deep selects Premium. The saved dispatch destination controls placement, subject to locality policy. For routine recon/tracing/extraction select class reconnaissance and normally omit tier."},
			"cwd":             {"type": "string", "description": "Optional absolute project working directory for the sub-agent. Use this for git/GitHub workflows so scoped tools run in the intended repository."},
			"path":            {"type": "string", "description": "Alias for cwd."},
			"intent":          {"type": "string", "description": "Optional concise human-facing reason for the delegation, shown in permission prompts."},
			"conversation_id": {"type": "string", "description": "Deprecated consistency check; must match the host-bound chat. Never establishes ownership."}
		}
	}`)
}

type dispatchArgs struct {
	Task           string   `json:"task"`
	Class          string   `json:"class"`
	Tools          []string `json:"tools"`
	Tier           string   `json:"tier"`
	Cwd            string   `json:"cwd"`
	Path           string   `json:"path"`
	Intent         string   `json:"intent"`
	ConversationID string   `json:"conversation_id"`
}

// tierForDispatch maps the model-facing "how much brain" knob onto a taxonomy
// tier. It expresses reasoning demand only — never location. Where the tier
// resolves (open vs cloud) is decided downstream by the user's locus mode via
// the saved dispatch assignment, not here. Omission leaves quality unset for
// the engine; unknown explicit values retain the historical Economy mapping.
func tierForDispatch(s string) config.Tier {
	return config.DispatchDifficultyTier(s)
}

func (dispatchCap) Execute(ctx context.Context, call *capabilities.Call) (*capabilities.Result, error) {
	var a dispatchArgs
	if err := json.Unmarshal(call.Args, &a); err != nil {
		return nil, fmt.Errorf("dispatch: parse args: %w", err)
	}
	if a.Task == "" {
		return nil, errors.New("dispatch: 'task' is required")
	}
	if strings.TrimSpace(call.ConversationID) == "" {
		return nil, errors.New("dispatch: trusted per-chat binding missing; host integration must supply chat identity")
	}
	if call.Svc.Tasks == nil {
		return nil, errors.New("dispatch: independent task service unavailable")
	}
	// Ownership comes only from the host-bound context.
	convID := call.ConversationID
	if a.ConversationID != "" && a.ConversationID != convID {
		return nil, errors.New("dispatch: conversation_id does not match the host-bound chat")
	}
	workDir := strings.TrimSpace(a.Cwd)
	if workDir == "" {
		workDir = strings.TrimSpace(a.Path)
	}
	if workDir == "" {
		workDir = call.WorkDir
	}
	task := config.Task(strings.TrimSpace(a.Class))
	if task == "" {
		task = config.TaskDispatch
	}
	if !config.ValidTask(task) {
		return nil, fmt.Errorf("dispatch: unknown task class %q", task)
	}
	readOnly := (dispatchCap{}).TierFor(call.Args) == capabilities.TierR
	if task == config.TaskReconnaissance && !readOnly {
		return nil, errors.New("dispatch: reconnaissance requires read-only grants")
	}
	res, err := call.Svc.Tasks.Launch(ctx, dispatch.Spec{
		Mode:     dispatch.Agentic,
		ReadOnly: readOnly,
		// Task class, not legacy role, owns routing.
		Role:           dispatch.RoleCoproc,
		RoutingTask:    task,
		Tier:           tierForDispatch(a.Tier),
		Task:           a.Task,
		Tools:          a.Tools,
		WorkDir:        workDir,
		ConversationID: convID,
		Interactive:    false,
	})
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(map[string]string{"task_id": res.ID, "child_id": res.ChildID, "status": res.Status})
	if err != nil {
		return nil, err
	}
	// Announce the accepted child on the launching turn so its tool row can
	// link to the tab. The tool loop supplies the originating tool-call ID.
	// This callback is not retained by the independently running task.
	if call.EmitProgress != nil {
		call.EmitProgress(agenttools.ProgressEvent{
			Kind: "started", SubAgentID: res.ChildID, SubAgentParentID: convID,
		})
	}
	return capabilities.NewTextResult(string(data)), nil
}

// agentGrantTiers maps every agent-surface tool name (display aliases and
// synonyms included) to its tier, for dispatch's dynamic permission. Built
// once — the built-in catalog is static.
var (
	grantTiersOnce sync.Once
	grantTiers     map[string]capabilities.Tier
)

func agentGrantTiers() map[string]capabilities.Tier {
	grantTiersOnce.Do(func() {
		grantTiers = map[string]capabilities.Tier{}
		reg := capabilities.NewRegistry(capabilities.Services{})
		Register(reg)
		aliases := AgentAliases()
		syns := CapabilitySynonyms()
		for _, c := range reg.ForSurface(capabilities.SurfaceAgent) {
			display := c.Name()
			if d, ok := aliases[c.Name()]; ok && d != "" {
				display = d
			}
			grantTiers[display] = c.Tier()
			if legacy, ok := c.(capabilities.LegacyAgentNamer); ok {
				for _, name := range legacy.LegacyAgentNames() {
					grantTiers[name] = c.Tier()
				}
			}
			for _, s := range syns[c.Name()] {
				grantTiers[s] = c.Tier()
			}
		}
	})
	return grantTiers
}

// stripHostPrefix removes a leading mcp__<host>__ segment, mirroring the
// server's grant-name normalization.
func stripHostPrefix(name string) string {
	if rest, ok := strings.CutPrefix(name, "mcp__"); ok {
		if idx := strings.Index(rest, "__"); idx >= 0 && rest[idx+2:] != "" {
			return rest[idx+2:]
		}
	}
	return name
}

// TierFor implements capabilities.ArgsTiered. A dispatch whose grant is all
// read-only built-ins stays R (no confirmation in any permission mode); any
// write-capable or unknown grant (MCP tools, typos) escalates the dispatch
// call itself to X so a human confirms ONCE — that approval authorizes the
// sub-agent's whole granted toolset for the run.
func (dispatchCap) TierFor(args json.RawMessage) capabilities.Tier {
	var a dispatchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return capabilities.TierX
	}
	if len(a.Tools) == 0 {
		return capabilities.TierR // least-privilege default grant: read-only
	}
	tiers := agentGrantTiers()
	for _, name := range a.Tools {
		if plain := stripHostPrefix(name); plain == "dispatch" || plain == "workflow" {
			return capabilities.TierX
		}
		t, ok := tiers[name]
		if !ok {
			t, ok = tiers[stripHostPrefix(name)]
		}
		if !ok || t != capabilities.TierR {
			return capabilities.TierX
		}
	}
	return capabilities.TierR
}
