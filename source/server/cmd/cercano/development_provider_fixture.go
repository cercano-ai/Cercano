//go:build developmentfixture && cercano_development_acceptance

// This file is compiled ONLY into explicit offline acceptance binaries.
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

func developmentProductionDefaults() developmentProductionDependencies {
	return developmentProductionDependencies{
		openSecrets: func() (secrets.Store, error) { return secrets.NewMemory(), nil },
		connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func(), error) {
			return localruntime.NewManager(), func() {}, nil
		},
		newOpenProvider: func(_ localruntime.Manager, cfg config.Config) (inference.Provider, error) {
			fixtureRecord("constructed", inference.Call{Model: cfg.OpenRuntime + ":" + cfg.LlamaServer.DefaultModel})
			b, _ := json.Marshal(map[string]any{"llama_enabled": cfg.LlamaServer.Enabled, "models": cfg.Models, "chat": cfg.TaskAssignment(config.TaskChat), "dispatch": cfg.TaskAssignment(config.TaskDispatch)})
			_ = os.WriteFile(filepath.Join(os.Getenv("CERCANO_FIXTURE_BARRIER"), "routing-config.json"), b, 0600)
			return &acceptanceProvider{}, nil
		},
	}
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
			return inference.Result{Blocks: []llm.Block{{Type: llm.BlockToolUse, ToolUseID: "acceptance-dispatch", ToolName: "dispatch", ToolInput: json.RawMessage(`{"task":"FIXTURE_CHILD complete the offline independent acceptance task","tools":["Read"]}`)}}, StopReason: "tool_use"}, nil
		}
	}
	return inference.Result{Blocks: []llm.Block{{Type: llm.BlockText, Text: "FIXTURE_OK " + kind}}, StopReason: "end_turn"}, nil
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
