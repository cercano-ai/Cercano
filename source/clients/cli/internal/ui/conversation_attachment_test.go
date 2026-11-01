package ui

import (
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"testing"
)

func TestAutomaticReplyDoesNotDuplicateOrModifyForeground(t *testing.T) {
	m := New(nil, false)
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.convID = "owner"
	m.ensureTaskSubscription()
	deliver := func(seq uint64, bg bool, message agentclient.StreamMsg) {
		m.applyConversationAttachment(conversationAttachmentMsg{parent: "owner", generation: m.taskSubscriptionGeneration, update: agentclient.ConversationUpdate{Local: !bg, Response: &proto.StreamProcessResponse{TurnEpoch: "host", TurnGeneration: 1, EventSequence: seq, BackgroundTurn: bg}, Message: &message}})
	}
	deliver(1, true, agentclient.StreamMsg{Type: agentclient.TypeToken, Token: "automatic "})
	m.mainChat().Apply(chatAssistantDeltaMsg{token: "foreground"})
	foreground := m.mainChat().streamingTextEntry()
	deliver(2, true, agentclient.StreamMsg{Type: agentclient.TypeToken, Token: "reply"})
	deliver(2, true, agentclient.StreamMsg{Type: agentclient.TypeToken, Token: "reply"})
	deliver(3, true, agentclient.StreamMsg{Type: agentclient.TypeDone, Final: "automatic reply"})
	deliver(4, false, agentclient.StreamMsg{Type: agentclient.TypeToken, Token: "duplicate foreground"})
	if foreground.Content != "foreground" {
		t.Fatalf("background corrupted foreground: %q", foreground.Content)
	}
	count := 0
	for _, entry := range m.mainChat().entries {
		if entry.Content == "automatic reply" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("automatic reply count=%d", count)
	}
}
func TestAutomaticReplyAttachmentPermissionAndChatFence(t *testing.T) {
	m := New(nil, false)
	m.convID = "owner"
	m.ensureTaskSubscription()
	generation := m.taskSubscriptionGeneration
	permission := agentclient.StreamMsg{Type: agentclient.TypePermissionRequired, ToolUseID: "approval", ToolName: "Edit", Tier: "W", ArgsJSON: "{}"}
	msg := conversationAttachmentMsg{parent: "owner", generation: generation, update: agentclient.ConversationUpdate{Response: &proto.StreamProcessResponse{TurnEpoch: "host", TurnGeneration: 1, BackgroundTurn: true}, Message: &permission}}
	m.applyConversationAttachment(msg)
	m.applyConversationAttachment(msg)
	if m.pendingConfirm == nil || m.pendingConfirm.tool.ToolUseID != "approval" || len(m.confirmQueue) != 0 {
		t.Fatal("approval missing or duplicated")
	}
	m.pendingConfirm = nil
	m.convID = "other"
	m.ensureTaskSubscription()
	m.applyConversationAttachment(msg)
	if m.pendingConfirm != nil {
		t.Fatal("approval crossed chat boundary")
	}
	m.convID = "owner"
	m.ensureTaskSubscription()
	m.applyConversationAttachment(msg)
	if m.pendingConfirm != nil {
		t.Fatal("old subscription became valid after switching back")
	}
}

func TestCompletedBackgroundReplayDoesNotDuplicateRestoredHistory(t *testing.T) {
	m := New(nil, false)
	m.convID = "owner"
	m.ensureTaskSubscription()
	m.resumeTurns = []agentclient.PersistedTurn{{ID: "saved-answer", Role: "assistant", Content: "automatic reply"}}
	m.mainChat().AppendEntry(&Entry{Role: RoleAssistant, Content: "automatic reply"})
	message := agentclient.StreamMsg{Type: agentclient.TypeDone, Final: "automatic reply"}
	m.applyConversationAttachment(conversationAttachmentMsg{parent: "owner", generation: m.taskSubscriptionGeneration, update: agentclient.ConversationUpdate{Response: &proto.StreamProcessResponse{PersistedTurnId: "saved-answer", TurnEpoch: "host", TurnGeneration: 1, EventSequence: 3, BackgroundTurn: true}, Message: &message}})
	count := 0
	for _, entry := range m.mainChat().entries {
		if entry.Content == "automatic reply" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("restored answer duplicated by replay: count=%d", count)
	}
}

func TestReconnectedBackgroundApprovalDoesNotOfferToReplayUserPrompt(t *testing.T) {
	m := New(nil, false)
	m.convID = "owner"
	m.ensureTaskSubscription()
	m.lastSubmittedPrompt = "unrelated original user request"
	permission := agentclient.StreamMsg{Type: agentclient.TypePermissionRequired, ToolUseID: "approval", ToolName: "Edit", Tier: "W", ArgsJSON: "{}"}
	m.applyConversationAttachment(conversationAttachmentMsg{parent: "owner", generation: m.taskSubscriptionGeneration, update: agentclient.ConversationUpdate{Response: &proto.StreamProcessResponse{TurnEpoch: "host", TurnGeneration: 1, BackgroundTurn: true}, Message: &permission}})
	m.connState = agentclient.ConnStateReconnecting
	m = send(t, m, connStateChangedMsg{state: agentclient.ConnStateConnected})
	if m.pendingConfirm == nil || m.pendingConfirm.stale {
		t.Fatal("background approval incorrectly offers to re-run the unrelated foreground request")
	}
}
