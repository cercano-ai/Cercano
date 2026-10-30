//go:build developmentfixture && cercano_development_acceptance

// This file is compiled ONLY into explicit offline acceptance binaries.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/localruntime"
	mcphost "cercano/source/server/internal/mcp_host"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/watchdog"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/devhostauth"
)

func developmentProductionDefaults() developmentProductionDependencies {
	d := developmentProductionDependencies{
		cloudBuilder: func(config.CloudProfile, string, ...cloudfactory.Options) (inference.Provider, error) {
			panic("unexpected acceptance cloud provider")
		},
		watchdog: server.WatchdogDependencies{
			OpenAudit: func(string) (watchdog.AuditLogger, error) { panic("unexpected acceptance watchdog audit") },
			OneShot:   func(context.Context, string) (string, error) { panic("unexpected acceptance watchdog inference") },
		},
		mcp: productionMCPDependencies{
			newManager: func(registry *agenttools.Registry, dir string, timeout time.Duration) *mcphost.Manager {
				// Only the validated fixture-owned stdio process may be started.
				// Keep the ordinary manager, registration and tool-call path.
				return mcphost.New(registry, dir, timeout)
			},
		},
		openSecrets: func() (secrets.Store, error) { return secrets.NewMemory(), nil },
		connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func() error, error) {
			return localruntime.NewManager(), func() error { return nil }, nil
		},
		newOpenProvider: func(_ localruntime.Manager, cfg config.Config) (inference.Provider, error) {
			fixtureRecord("constructed", inference.Call{Model: cfg.OpenRuntime + ":" + cfg.LlamaServer.DefaultModel})
			b, _ := json.Marshal(map[string]any{"llama_enabled": cfg.LlamaServer.Enabled, "models": cfg.Models, "chat": cfg.TaskAssignment(config.TaskChat), "dispatch": cfg.TaskAssignment(config.TaskDispatch)})
			_ = os.WriteFile(filepath.Join(os.Getenv("CERCANO_FIXTURE_BARRIER"), "routing-config.json"), b, 0600)
			return &acceptanceProvider{}, nil
		},
	}
	// Tagged offline process only: construct every non-dialing dependency before
	// entering the enabled scope. The launcher and outer test remain disabled.
	return d
}

type acceptanceProvider struct{}

// The production worker requires the owning provider to confirm live capacity.
// This is synthetic capacity for the synthetic provider, not a model launch.
func (*acceptanceProvider) RuntimeContext(context.Context, string, bool) (llm.RuntimeContext, error) {
	return llm.RuntimeContext{Window: 32768, InstanceID: "offline-compiled-acceptance"}, nil
}

func (*acceptanceProvider) Name() string { return "llama_server" }
func (*acceptanceProvider) Capabilities() inference.Capabilities {
	return inference.Capabilities{SupportsTools: true}
}
func fixtureRecord(event string, req inference.Call) {
	b, _ := json.Marshal(map[string]any{"event": event, "pid": os.Getpid(), "conversation": req.ConversationID, "request": req})
	f, e := os.OpenFile(filepath.Join(os.Getenv("CERCANO_FIXTURE_BARRIER"), "provider.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e == nil {
		defer f.Close()
		f.Write(append(b, '\n'))
	}
}
func (*acceptanceProvider) Chat(ctx context.Context, req inference.Call) (inference.Result, error) {
	fixtureRecord("call", req)
	text := ""
	for _, m := range req.Messages {
		if m.Role == llm.RoleUser {
			for _, b := range m.Blocks {
				if b.Type == llm.BlockText {
					text = b.Text
				}
			}
		}
	}
	if strings.HasPrefix(text, "<host:task-completions>") {
		fixtureRecord("completion-notice", req)
		if fixtureWorkerDelegation() {
			if err := fixtureWait(ctx, "final"); err != nil {
				return inference.Result{}, err
			}
		}
	}
	// Review with a tool grant is an ordinary worker-local agentic dispatch,
	// not the host's independent-task launch capability.
	if strings.Contains(text, "FIXTURE_WORKER_DELEGATE") {
		seen := false
		for _, m := range req.Messages {
			for _, b := range m.Blocks {
				if b.Type == llm.BlockToolResult && b.ToolUseRef == "acceptance-worker-review" {
					seen = true
				}
			}
		}
		if !seen {
			fixtureRecord("worker-delegation-request", req)
			return inference.Result{Blocks: []llm.Block{{Type: llm.BlockToolUse, ToolUseID: "acceptance-worker-review", ToolName: "review", ToolInput: json.RawMessage(`{"claim":"FIXTURE_CHILD descendant execution","tools":["Read"]}`)}}, StopReason: "tool_use"}, nil
		}
		fixtureRecord("worker-delegation-result", req)
	}
	// Worker prompts contain the marker in the independently dispatched task.
	kind := ""
	if strings.Contains(text, "FIXTURE_CHILD") {
		kind = "child"
	} else if strings.Contains(text, "FIXTURE_HOLD") {
		kind = "foreground"
	}
	if kind != "" {
		fixtureRecord(kind+"-entered", req)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			if _, e := os.Stat(filepath.Join(os.Getenv("CERCANO_FIXTURE_BARRIER"), kind+".release")); e == nil {
				break
			}
			select {
			case <-ctx.Done():
				return inference.Result{}, ctx.Err()
			case <-ticker.C:
			}
		}
		fixtureRecord(kind+"-released", req)
	}
	if strings.Contains(text, "FIXTURE_MCP") {
		seen := false
		for _, m := range req.Messages {
			if m.Role == llm.RoleUser {
				for _, b := range m.Blocks {
					if b.Type == llm.BlockText && strings.Contains(b.Text, "FIXTURE_MCP") {
						seen = false
					}
				}
			}
			for _, b := range m.Blocks {
				if b.Type == llm.BlockToolResult && b.ToolUseRef == "acceptance-mcp" {
					seen = true
				}
			}
		}
		if !seen {
			return inference.Result{Blocks: []llm.Block{{Type: llm.BlockToolUse, ToolUseID: "acceptance-mcp", ToolName: "mcp__fixture__echo", ToolInput: json.RawMessage(`{"text":"FIXTURE_MCP_RESULT"}`)}}, StopReason: "tool_use"}, nil
		}
		fixtureRecord("mcp-result-request", req)
	}
	if strings.Contains(text, "FIXTURE_DISPATCH") {
		seen := false
		for _, m := range req.Messages {
			for _, b := range m.Blocks {
				if b.Type == llm.BlockToolResult {
					seen = true
				}
			}
		}
		if !seen {
			input := json.RawMessage(`{"task":"FIXTURE_CHILD complete the offline independent acceptance task","tools":["Read"]}`)
			if fixtureWorkerDelegation() {
				input = json.RawMessage(`{"task":"FIXTURE_WORKER_DELEGATE use review to delegate the claim, then report the result","tools":["review","Read"]}`)
			}
			return inference.Result{Blocks: []llm.Block{{Type: llm.BlockToolUse, ToolUseID: "acceptance-dispatch", ToolName: "dispatch", ToolInput: input}}, StopReason: "tool_use"}, nil
		}
	}
	return inference.Result{Blocks: []llm.Block{{Type: llm.BlockText, Text: "FIXTURE_OK " + kind}}, StopReason: "end_turn"}, nil
}
func fixtureWorkerDelegation() bool {
 _, err := os.Stat(filepath.Join(os.Getenv("CERCANO_FIXTURE_BARRIER"), "worker-delegation.enabled"))
 return err == nil
}

// fixtureWait holds the synthetic completion callback before its final history
// write. Cancellation is bounded by the owning request; the harness has a
// separate deadline and only ever releases barriers in its private directory.
func fixtureWait(ctx context.Context, kind string) error {
	fixtureRecord(kind+"-entered", inference.Call{})
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(filepath.Join(os.Getenv("CERCANO_FIXTURE_BARRIER"), kind+".release")); err == nil {
			fixtureRecord(kind+"-released", inference.Call{})
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (p *acceptanceProvider) StreamChat(ctx context.Context, req inference.Call) (inference.Stream, error) {
	r, e := p.Chat(ctx, req)
	if e != nil {
		return nil, e
	}
	events := []llm.StreamEvent{{Type: llm.EventMessageStart}}
	for _, b := range r.Blocks {
		if b.Type == llm.BlockText {
			events = append(events, llm.StreamEvent{Type: llm.EventTextDelta, TextDelta: b.Text})
		} else {
			events = append(events, llm.StreamEvent{Type: llm.EventToolUseStart, ToolUseID: b.ToolUseID, ToolName: b.ToolName}, llm.StreamEvent{Type: llm.EventToolUseInputDelta, TextDelta: string(b.ToolInput)}, llm.StreamEvent{Type: llm.EventToolUseStop})
		}
	}
	events = append(events, llm.StreamEvent{Type: llm.EventMessageStop, StopReason: r.StopReason})
	return &acceptanceStream{events: events}, nil
}

type acceptanceStream struct{ events []llm.StreamEvent }

func (s *acceptanceStream) Next() (llm.StreamEvent, bool, error) {
	if len(s.events) == 0 {
		return llm.StreamEvent{}, false, nil
	}
	e := s.events[0]
	s.events = s.events[1:]
	return e, true, nil
}
func (*acceptanceStream) Close() error { return nil }

func newProductionDevelopmentHost(ctx context.Context, o developmentHostOptions, auth *devhostauth.Auth) (*server.Server, *preparedShutdown, error) {
	// Validate the one shared, explicitly selected authority before enabling
	// credential admission. No ambient MCP definitions or executables qualify.
	barrier := os.Getenv("CERCANO_FIXTURE_BARRIER")
	if err := developmentPrivatePath(barrier, true); err != nil {
		return nil, nil, err
	}
	fixture := filepath.Join(barrier, "mcp-fixture")
	if err := developmentPrivatePath(fixture, false); err != nil {
		return nil, nil, err
	}
	mcpConfig, err := mcphost.LoadConfig(o.State)
	if err != nil {
		return nil, nil, err
	}
	entry, ok := mcpConfig.Servers["fixture"]
	if !ok || len(mcpConfig.Servers) != 1 || entry.Command != fixture || len(entry.Args) != 0 || len(entry.Env) != 1 || entry.Env["CERCANO_DISABLE_CREDENTIALS"] != "1" {
		return nil, nil, fmt.Errorf("acceptance requires only the fixed fixture MCP definition with credentials disabled")
	}
	d := developmentProductionDefaults()
	old, existed := os.LookupEnv("CERCANO_DISABLE_CREDENTIALS")
	defer func() {
		if existed {
			_ = os.Setenv("CERCANO_DISABLE_CREDENTIALS", old)
		} else {
			_ = os.Unsetenv("CERCANO_DISABLE_CREDENTIALS")
		}
	}()
	_ = os.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	return newProductionDevelopmentHostWithDependencies(ctx, o, auth, d)
}
