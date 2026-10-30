package acp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"cercano/source/server/pkg/agentclient"
)

// SharedClient is the narrow agentclient surface used by the adapter.
type SharedClient interface {
	SubscribeEvents(context.Context) (<-chan agentclient.AgentEvent, error)
	StreamChat(context.Context, string, string, string, ...agentclient.InlineImage) (<-chan agentclient.StreamMsg, error)
	AttachConversation(context.Context, string) (<-chan agentclient.ConversationUpdate, error)
	AllowToolCall(context.Context, string, string) error
	DenyToolCall(context.Context, string, string) error
	Close() error
}

type AgentBackend struct {
	client     SharedClient
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	eventsOnce sync.Once
	eventsErr  error
}

func NewAgentBackend(ctx context.Context, c SharedClient) *AgentBackend {
	ctx, cancel := context.WithCancel(ctx)
	return &AgentBackend{client: c, ctx: ctx, cancel: cancel}
}
func (b *AgentBackend) NewSession(ctx context.Context, cwd string) (string, error) {
	// Only SubscribeEvents participates in shared-daemon idle shutdown.
	b.eventsOnce.Do(func() {
		var events <-chan agentclient.AgentEvent
		events, b.eventsErr = b.client.SubscribeEvents(b.ctx)
		if b.eventsErr != nil {
			return
		}
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			for {
				select {
				case <-b.ctx.Done():
					return
				case _, ok := <-events:
					if !ok {
						return
					}
				}
			}
		}()
	})
	if b.eventsErr != nil {
		return "", fmt.Errorf("retain shared daemon: %w", b.eventsErr)
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	// The server accepts caller-assigned conversation IDs. Retain that same ID
	// on ACP, all prompts, attachments, and permission decisions.
	sid := "acp-" + hex.EncodeToString(id[:])
	updates, err := b.client.AttachConversation(b.ctx, sid)
	if err != nil {
		return "", fmt.Errorf("attach shared conversation: %w", err)
	}
	// Drain the passive observer; prompt output comes solely from its
	// lossless initiating stream, never duplicated from this observer.
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			select {
			case <-b.ctx.Done():
				return
			case _, ok := <-updates:
				if !ok {
					return
				}
			}
		}
	}()
	return sid, nil
}
func (b *AgentBackend) Prompt(ctx context.Context, sid, cwd, text string) (<-chan Event, error) {
	msgs, err := b.client.StreamChat(ctx, sid, text, cwd)
	if err != nil {
		return nil, err
	}
	out := make(chan Event)
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer close(out)
		for {
			var m agentclient.StreamMsg
			select {
			case <-ctx.Done():
				return
			case <-b.ctx.Done():
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}
				m = msg
			}
			e := Event{Text: m.Token, ToolID: m.ToolUseID, ToolName: m.ToolName, Input: m.ArgsJSON, Failed: m.IsError, Err: m.Err}
			switch m.Type {
			case agentclient.TypeToken:
				e.Kind = "text"
			case agentclient.TypeProgress:
				e.Kind = "thought"
				e.Text = m.Note
			case agentclient.TypeDone:
				e.Kind = "done"
				e.Text = m.Final
			case agentclient.TypeError:
				e.Kind = "error"
			case agentclient.TypeToolUseStart:
				e.Kind = "tool"
			case agentclient.TypeToolUseStop:
				e.Kind = "tool_input"
				e.Text = m.ArgsSummary
			case agentclient.TypeToolExecStart:
				e.Kind = "tool_running"
			case agentclient.TypeToolExecComplete:
				e.Kind = "tool_done"
				e.Text = m.Summary
			case agentclient.TypePermissionRequired:
				e.Kind = "permission"
			case agentclient.TypeAuthentication:
				e.Kind = "error"
				e.Err = fmt.Errorf("authentication required: authenticate in Cercano CLI, then retry this prompt")
			case agentclient.TypeRolloverOffered:
				e.Kind = "error"
				e.Err = fmt.Errorf("session rollover requires Cercano CLI; start a new ACP session or manage rollover there")
			default:
				continue
			}
			select {
			case out <- e:
			case <-ctx.Done():
				return
			case <-b.ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
func (b *AgentBackend) Permission(ctx context.Context, sid, tool string, allow bool) error {
	if allow {
		return b.client.AllowToolCall(ctx, sid, tool)
	}
	return b.client.DenyToolCall(ctx, sid, tool)
}
func (b *AgentBackend) Close() error { b.cancel(); err := b.client.Close(); b.wg.Wait(); return err }
