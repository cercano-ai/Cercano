package ui

import (
	"cercano/source/server/pkg/agentclient"
	"context"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestTakeoverConfirmationExplicitOnly(t *testing.T) {
	st, err := status.New(codes.AlreadyExists, "attached").WithDetails(&errdetails.ErrorInfo{Reason: agentclient.TerminalConflictReason, Metadata: map[string]string{"terminal": "old"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"enter", "n", "esc", "y"} {
		t.Run(key, func(t *testing.T) {
			m := New(nil, false)
			m.taskSubscriptionContext = context.Background()
			m.applyConversationAttachment(conversationAttachmentMsg{parent: m.convID, generation: m.taskSubscriptionGeneration, update: agentclient.ConversationUpdate{Err: st.Err()}})
			if m.pendingConfirm == nil {
				t.Fatal("missing confirmation")
			}
			next, cmd := m.resolveConfirmKey(key)
			if next.pendingConfirm != nil {
				t.Fatal("confirmation not resolved")
			}
			if (cmd != nil) != (key == "y") {
				t.Fatalf("unexpected takeover command for %s", key)
			}
		})
	}
}
