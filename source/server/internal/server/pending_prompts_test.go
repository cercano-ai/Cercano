package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/llm"
	"cercano/source/server/pkg/proto"
)

func TestPendingPromptsSnapshotIsolationAndCleanup(t *testing.T) {
	var h pendingPromptHub
	changes, detach := h.subscribe("owner")
	<-changes
	prompt := &proto.PermissionRequired{ToolUseId: "tool", ToolName: "original"}
	forget := h.publish("owner", prompt)
	prompt.ToolName = "mutated"
	<-changes
	if got := h.snapshot("foreign"); len(got) != 0 {
		t.Fatal("foreign prompt leaked")
	}
	snapshot := h.snapshot("owner")
	if len(snapshot) != 1 || snapshot[0].ToolName != "original" {
		t.Fatalf("snapshot=%v", snapshot)
	}
	snapshot[0].ToolName = "mutated snapshot"
	if h.snapshot("owner")[0].ToolName != "original" {
		t.Fatal("snapshot aliases live prompt")
	}
	detach() // An observer disconnect does not erase the still-pending prompt.
	if len(h.snapshot("owner")) != 1 {
		t.Fatal("disconnect lost pending prompt")
	}
	forget()
	forget()
	if len(h.snapshot("owner")) != 0 || len(h.chats) != 0 {
		t.Fatal("resolved prompt retained")
	}
}

func TestAttachReplaysPendingApprovalWithoutGrantingIt(t *testing.T) {
	srv, _ := newServerWithStore(t)
	gated := &gatedExecTool{}
	registry := agenttools.NewRegistry()
	registry.MustRegister(gated)
	srv.SetToolRegistry(registry)
	srv.SetPermissions(agent.NewStaticPermissionStore(agent.ModePermissive), agent.NewPendingDecisions())
	srv.SetCloudLLMProvider(&scriptedProvider{caps: inferenceCapabilities(), scripts: [][]llm.Block{
		{{Type: llm.BlockToolUse, ToolUseID: "retained", ToolName: "gated_work", ToolInput: json.RawMessage(`{}`)}},
		{{Type: llm.BlockText, Text: "done"}},
	}})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	published := make(chan struct{}, 1)
	owner := &immediatePermissionStream{ctx: ctx, reply: func(*proto.PermissionRequired) { published <- struct{}{} }}
	done := make(chan error, 1)
	go func() {
		done <- srv.streamProcessRequestWithToolLoop(&proto.ProcessRequestRequest{ConversationId: "owner", Input: "work"}, owner)
	}()
	select {
	case <-published:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for attempt := 0; attempt < 2; attempt++ {
		observerCtx, disconnect := context.WithCancel(ctx)
		replayed := make(chan string, 1)
		observer := &immediatePermissionStream{ctx: observerCtx, reply: func(prompt *proto.PermissionRequired) {
			// A retained foreground request does not become background when a
			// replacement observer attaches. The UI must not filter it out.
			if prompt.BackgroundTurn || prompt.TurnEpoch == "" || prompt.TurnGeneration == 0 {
				t.Errorf("replayed foreground prompt lost its identity: %v", prompt)
			}
			replayed <- prompt.ToolUseId
			disconnect()
		}}
		err := srv.AttachConversation(&proto.AttachConversationRequest{ConversationId: "owner"}, observer)
		disconnect()
		if err != context.Canceled {
			t.Fatalf("attach: %v", err)
		}
		select {
		case id := <-replayed:
			if id != "retained" {
				t.Fatal(id)
			}
		default:
			t.Fatal("missing pending prompt")
		}
		if gated.execCount() != 0 {
			t.Fatal("attach or disconnect implicitly approved the action")
		}
	}
	if !srv.permBroker.Resolve("owner", "retained", agent.Decision{Allow: true}) {
		t.Fatal("reconnected approval not accepted")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if gated.execCount() != 1 {
		t.Fatalf("executions=%d", gated.execCount())
	}
	if len(srv.pendingPrompts.snapshot("owner")) != 0 {
		t.Fatal("settled prompt remains replayable")
	}
}
