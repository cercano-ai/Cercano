package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"cercano/source/server/internal/acp/v1"
)

type session struct {
	cwd        string
	cancel     context.CancelFunc
	pending    bool
	delivering <-chan struct{} // non-nil only while the pump owns the terminal write
}

func (s *server) handle(f frame) {
	bad := func(message string) { s.reply(f.ID, nil, &Error{-32602, message}) }
	if f.Method == "initialize" {
		var p v1.InitializeRequest
		if json.Unmarshal(f.Params, &p) != nil || p.ProtocolVersion == nil {
			bad("protocolVersion is required")
			return
		}
		if s.initialized {
			bad("already initialized")
			return
		}
		s.initialized = true
		s.reply(f.ID, v1.InitializeResponse{ProtocolVersion: 1, AgentCapabilities: map[string]any{"loadSession": false, "promptCapabilities": map[string]bool{"image": false, "audio": false, "embeddedContext": false}, "mcpCapabilities": map[string]bool{"http": false, "sse": false}}, AuthMethods: []any{}}, nil)
		return
	}
	if !s.initialized {
		s.reply(f.ID, nil, &Error{-32600, "initialize must complete first"})
		return
	}
	switch f.Method {
	case "session/new":
		if len(f.ID) == 0 {
			return
		}
		var p v1.NewSessionRequest
		if json.Unmarshal(f.Params, &p) != nil || !filepath.IsAbs(p.CWD) || p.MCPServers == nil {
			bad("absolute cwd and mcpServers array are required")
			return
		}
		if len(p.MCPServers) > 0 {
			bad("client mcpServers are not supported; configure tools in the shared daemon")
			return
		}
		if len(p.AdditionalDirectories) > 0 {
			bad("additionalDirectories are not supported")
			return
		}
		if s.backend == nil {
			b, err := s.connect(s.ctx)
			if err != nil {
				s.reply(f.ID, nil, &Error{-32603, "connect shared daemon: " + err.Error()})
				return
			}
			s.backend = b
		}
		id, err := s.backend.NewSession(s.ctx, p.CWD)
		if err != nil {
			s.reply(f.ID, nil, &Error{-32603, err.Error()})
			return
		}
		s.mu.Lock()
		s.sessions[id] = &session{cwd: p.CWD}
		s.mu.Unlock()
		s.reply(f.ID, v1.NewSessionResponse{SessionID: id}, nil)
	case "session/prompt", "session/cancel":
		var p v1.PromptRequest
		if json.Unmarshal(f.Params, &p) != nil || p.SessionID == "" {
			bad("sessionId is required")
			return
		}
		s.mu.Lock()
		session := s.sessions[p.SessionID]
		if session == nil {
			s.mu.Unlock()
			bad("session does not belong to this connection")
			return
		}
		if f.Method == "session/cancel" {
			if session.cancel != nil {
				session.cancel()
			}
			s.mu.Unlock()
			s.reply(f.ID, struct{}{}, nil)
			return
		}
		if len(f.ID) == 0 {
			s.mu.Unlock()
			return
		}
		// A final frame may already be visible to the peer before Write returns.
		// Join only an in-progress terminal write, never a queued completion.
		// Do not hold mu across IO: cancellation and permission cleanup need it.
		for session.delivering != nil {
			done := session.delivering
			s.mu.Unlock()
			select {
			case <-s.ctx.Done():
				return
			case <-done:
			}
			s.mu.Lock()
		}
		if s.ctx.Err() != nil {
			s.mu.Unlock()
			return
		}
		if session.pending {
			s.mu.Unlock()
			bad("a prompt is already running or its final response is pending for this session")
			return
		}
		if p.Prompt == nil {
			s.mu.Unlock()
			bad("prompt array is required")
			return
		}
		var parts []string
		for _, b := range p.Prompt {
			switch {
			case b.Type == "text" && b.Text != nil:
				parts = append(parts, *b.Text)
			case b.Type == "resource_link" && b.Name != nil && b.URI != nil:
				// Forward reference metadata only. Resolving it remains the backend's
				// responsibility under the shared daemon's normal tool policy.
				parts = append(parts, fmt.Sprintf("Resource reference: name=%q uri=%q", *b.Name, *b.URI))
			default:
				s.mu.Unlock()
				bad("prompt content must be text or a resource_link with name and uri strings")
				return
			}
		}
		if s.outstanding >= maxOutstandingPrompts {
			s.mu.Unlock()
			bad(fmt.Sprintf("outstanding prompt limit (%d) reached; wait for final responses before retrying", maxOutstandingPrompts))
			return
		}
		ctx, cancel := context.WithCancel(s.ctx)
		session.pending = true
		s.outstanding++
		session.cancel = cancel
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer cancel()
			result, err := s.prompt(ctx, p.SessionID, session.cwd, strings.Join(parts, "\n"))
			s.mu.Lock()
			session.cancel = nil
			s.mu.Unlock()
			s.finishPrompt(session, f.ID, result, err)
		}()
	default:
		s.reply(f.ID, nil, &Error{-32601, "method not implemented: " + f.Method})
	}
}
func (s *server) update(ctx context.Context, sid string, u v1.Update) {
	s.writeContext(ctx, notification{"2.0", "session/update", v1.SessionNotification{SessionID: sid, Update: u}})
}
func (s *server) prompt(ctx context.Context, sid, cwd, text string) (any, *Error) {
	cancelled := func() (any, *Error) { return v1.PromptResponse{StopReason: "cancelled"}, nil }
	events, err := s.backend.Prompt(ctx, sid, cwd, text)
	if err != nil {
		if ctx.Err() != nil {
			return cancelled()
		}
		return nil, &Error{-32603, err.Error()}
	}
	streamed := false
	tools := map[string]bool{}
	for {
		if ctx.Err() != nil {
			return cancelled()
		}
		select {
		case <-ctx.Done():
			return cancelled()
		case e, ok := <-events:
			if ctx.Err() != nil {
				return cancelled()
			}
			if !ok {
				if ctx.Err() != nil {
					return cancelled()
				}
				return nil, &Error{-32603, "backend stream ended without completion; retry the prompt"}
			}
			u := v1.Update{ToolCallID: e.ToolID}
			switch e.Kind {
			case "text", "thought":
				u.SessionUpdate = "agent_message_chunk"
				if e.Kind == "thought" {
					u.SessionUpdate = "agent_thought_chunk"
				} else {
					streamed = true
				}
				u.Content = v1.TextContent{Type: "text", Text: e.Text}
			case "tool", "tool_input", "tool_running", "tool_done":
				if e.ToolID == "" {
					return nil, &Error{-32603, "backend omitted tool call ID"}
				}
				u.SessionUpdate = "tool_call_update"
				if !tools[e.ToolID] {
					tools[e.ToolID] = true
					u.SessionUpdate = "tool_call"
					u.Title = e.ToolName
					if u.Title == "" {
						u.Title = e.ToolID
					}
				}
				switch e.Kind {
				case "tool":
					u.Status = "pending"
				case "tool_input":
					u.RawInput = e.Text
				case "tool_running":
					u.Status = "in_progress"
				case "tool_done":
					u.Status = "completed"
					if e.Failed {
						u.Status = "failed"
					}
					u.RawOutput = e.Text
					u.Content = []v1.ToolContent{{Type: "content", Content: v1.TextContent{Type: "text", Text: e.Text}}}
				}
			case "permission":
				if e.ToolID == "" {
					return nil, &Error{-32603, "backend omitted permission tool ID"}
				}
				if err := s.permission(ctx, sid, e); err != nil {
					if ctx.Err() != nil {
						return cancelled()
					}
					return nil, &Error{-32603, err.Error()}
				}
				continue
			case "done":
				if !streamed && e.Text != "" {
					s.update(ctx, sid, v1.Update{SessionUpdate: "agent_message_chunk", Content: v1.TextContent{Type: "text", Text: e.Text}})
				}
				if ctx.Err() != nil {
					return cancelled()
				}
				return v1.PromptResponse{StopReason: "end_turn"}, nil
			case "error":
				if e.Err == nil {
					e.Err = fmt.Errorf("backend failed; retry in Cercano CLI")
				}
				return nil, &Error{-32603, e.Err.Error()}
			default:
				continue
			}
			s.update(ctx, sid, u)
		}
	}
}
func (s *server) permission(ctx context.Context, sid string, e Event) error {
	s.mu.Lock()
	s.sequence++
	id := fmt.Sprintf("acp-permission-%d", s.sequence)
	ch := make(chan frame, 1)
	s.waits[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.waits, id); s.mu.Unlock() }()
	var input any
	if json.Unmarshal([]byte(e.Input), &input) != nil {
		input = e.Input
	}
	s.writeContext(ctx, request{"2.0", id, "session/request_permission", v1.RequestPermissionRequest{SessionID: sid, ToolCall: v1.ToolCallUpdate{ToolCallID: e.ToolID, Title: e.ToolName, Status: "pending", RawInput: input}, Options: []v1.PermissionOption{{OptionID: "allow_once", Name: "Allow once", Kind: "allow_once"}, {OptionID: "reject_once", Name: "Reject once", Kind: "reject_once"}}}})
	allow := false
	var invalid bool
	select {
	case <-ctx.Done():
	case f := <-ch:
		var p v1.RequestPermissionResponse
		if len(f.Error) > 0 || json.Unmarshal(f.Result, &p) != nil {
			invalid = true
		} else if p.Outcome.Outcome == "selected" && p.Outcome.OptionID == "allow_once" {
			allow = true
		} else if !(p.Outcome.Outcome == "cancelled" || (p.Outcome.Outcome == "selected" && p.Outcome.OptionID == "reject_once")) {
			invalid = true
		}
	}
	// Cancellation wins even if a queued allow response raced it. A cancelled
	// context cannot carry the denial RPC, so cleanup has a short independent TTL.
	if ctx.Err() != nil {
		allow = false
	}
	decisionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.backend.Permission(decisionCtx, sid, e.ToolID, allow); err != nil {
		return fmt.Errorf("resolve tool permission: %w", err)
	}
	if invalid {
		return fmt.Errorf("invalid permission response: tool denied; retry the prompt")
	}
	return ctx.Err()
}
