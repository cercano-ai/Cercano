package ui

import (
	"context"
	"testing"
	"time"

	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
)

// Called only by the explicit disposable ordinary-host subprocess harness.
// This exercises UI commands and real authenticated transport, not a PTY.
func verifyProductionTakeoverConfirmation(t *testing.T, ctx context.Context, owner, destination DevHostSelection) {
	t.Helper()
	client, err := owner.Dial(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	observer, err := owner.Dial(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	target := func() string {
		t.Helper()
		_, err := observer.AttachConversation(ctx, owner.Conversation)
		got := agentclient.TakeoverTarget(err)
		if got == "" {
			t.Fatalf("expected structured terminal conflict, got %v", err)
		}
		return got
	}
	original := target()
	m := New(client, false).WithDevHost(destination, "acceptance")
	m.convID = owner.Conversation
	attachCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	m.taskSubscriptionContext = attachCtx
	for _, key := range []string{"enter", "n", "esc", "y"} {
		msg := attachMainConversationCmd(attachCtx, client, m.convID, m.taskSubscriptionGeneration)().(conversationAttachmentMsg)
		if agentclient.TakeoverTarget(msg.update.Err) != original {
			t.Fatalf("%s: lost original conflict: %v", key, msg.update.Err)
		}
		m.applyConversationAttachment(msg)
		if m.pendingConfirm == nil {
			t.Fatal("missing takeover confirmation")
		}
		next, cmd := m.resolveConfirmKey(key)
		m = next
		if m.pendingConfirm != nil {
			t.Fatal("confirmation not dismissed")
		}
		if key != "y" {
			if cmd != nil {
				t.Fatalf("%s issued mutation", key)
			}
			if target() != original {
				t.Fatalf("%s replaced terminal", key)
			}
			continue
		}
		if cmd == nil {
			t.Fatal("yes did not issue takeover")
		}
		// The observer command waits for a turn event, not just enrollment. Watch
		// independent attachment conflicts for acknowledged replacement, then cancel
		// and join the command; no sleeps are used as success assertions.
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		deadline := time.NewTimer(3 * time.Second)
		ticker := time.NewTicker(10 * time.Millisecond)
		replaced := false
		for !replaced {
			select {
			case <-deadline.C:
				t.Fatal("yes did not replace terminal")
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-ticker.C:
				replaced = target() != original
			}
		}
		ticker.Stop()
		deadline.Stop()
		cancel()
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("attachment command did not stop")
		}
	}
	// Ordinary resume must not route back to A, even though A remains live.
	selected, err := reopenDevHost(ctx, destination, owner.Conversation)
	if err == nil || selected.Identity.Host != destination.Identity.Host {
		t.Fatalf("live owner resume escaped destination: %+v %v", selected, err)
	}
	if m.devHost.defaultSelection.Identity.Host != destination.Identity.Host {
		t.Fatal("resume changed default host")
	}
}
