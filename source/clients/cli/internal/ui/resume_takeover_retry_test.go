package ui

import (
	"cercano/source/server/pkg/proto"
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/agentclient"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Exercise the real resume reducer without executing history/network commands.
func TestResumeSameConversationAfterDecliningTakeover(t *testing.T) {
	isolateOrdinaryUI(t)
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	st, err := status.New(codes.AlreadyExists, "attached").WithDetails(&errdetails.ErrorInfo{Reason: agentclient.TerminalConflictReason, Metadata: map[string]string{"terminal": "owner-A"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"n", "esc", "enter"} {
		t.Run(key, func(t *testing.T) {
			m := New(&agentclient.Client{}, false)
			m.convID = "B"
			m.ensureTaskSubscription() // Commands are deliberately not run against this inert client.
			oldB := m.taskSubscriptionContext
			next, _ := m.Update(resumeRequestedMsg{ConversationID: "A"})
			m = next.(Model)
			if oldB.Err() == nil {
				t.Fatal("B subscription was not canceled")
			}
			oldCtx, oldGen := m.taskSubscriptionContext, m.taskSubscriptionGeneration
			conflict := conversationAttachmentMsg{parent: "A", generation: oldGen, update: agentclient.ConversationUpdate{Err: st.Err()}}
			m.applyConversationAttachment(conflict)
			if m.pendingConfirm == nil {
				t.Fatal("first resume did not offer takeover")
			}
			m, cmd := m.resolveConfirmKey(key)
			if cmd != nil || m.pendingConfirm != nil {
				t.Fatal("decline issued a command or left confirmation open")
			}
			// An ordinary update must not silently attach or take over after declining.
			next, _ = m.Update(struct{}{})
			m = next.(Model)
			if m.taskSubscriptionGeneration != oldGen || m.pendingConfirm != nil {
				t.Fatal("implicit retry after decline")
			}
			next, _ = m.Update(resumeRequestedMsg{ConversationID: "A"})
			m = next.(Model)
			defer m.taskSubscriptionCancel()
			if m.taskSubscriptionGeneration <= oldGen {
				t.Fatalf("same-conversation explicit resume did not retry attachment: generation stayed %d, old context canceled=%t", oldGen, oldCtx.Err() != nil)
			}
			if oldCtx.Err() == nil {
				t.Fatal("retry did not cancel previous subscription")
			}
			if m.applyConversationAttachment(conflict) != nil || m.pendingConfirm != nil {
				t.Fatal("stale conflict offered takeover")
			}
			if m.applyConversationAttachment(conversationAttachmentMsg{parent: "A", generation: oldGen, attached: true}) != nil {
				t.Fatal("stale ack started observers")
			}
			conflict.generation = m.taskSubscriptionGeneration
			m.applyConversationAttachment(conflict)
			if m.pendingConfirm == nil {
				t.Fatal("explicit retry did not offer takeover again")
			}
		})
	}
}

// The only service used here is an ephemeral loopback gRPC peer. It has no
// credentials, model, store, or lifecycle work to take over.
type retryAttachmentPeer struct {
	closeSessionPeer
	attempts  atomic.Int32
	takeovers atomic.Int32
}

func (p *retryAttachmentPeer) AttachConversation(_ *proto.AttachConversationRequest, stream proto.Agent_AttachConversationServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	expected := md.Get(agentclient.TerminalTakeoverMetadata)
	if len(expected) == 0 {
		attempt := p.attempts.Add(1)
		st, _ := status.New(codes.AlreadyExists, "attached").WithDetails(&errdetails.ErrorInfo{Reason: agentclient.TerminalConflictReason, Metadata: map[string]string{"terminal": fmt.Sprintf("owner-%d", attempt)}})
		return st.Err()
	}
	p.takeovers.Add(1)
	if len(expected) != 1 || expected[0] != "owner-2" {
		return status.Error(codes.FailedPrecondition, "stale takeover target")
	}
	if err := stream.SendHeader(metadata.Pairs("cercano-terminal-attached", "true")); err != nil {
		return err
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}
func TestResumeDeclineRetryAttachmentTransport(t *testing.T) {
	isolateOrdinaryUI(t)
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	peer := &retryAttachmentPeer{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	proto.RegisterAgentServer(server, peer)
	go server.Serve(listener)
	defer server.Stop()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, err := agentclient.DialPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: listener.Addr().String(), HostInstance: "test", Build: "build", Contract: "contract", SchemaVersion: 2, ConversationID: "synthetic", OwnershipEpoch: "epoch"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	m := New(client, false)
	m.convID = "B"
	m.ensureTaskSubscription()
	// Execute attachment commands only, not history hydration commands.
	m, _ = m.beginProgressiveResume("synthetic")
	cmd := m.ensureTaskSubscription()
	m.applyConversationAttachment(cmd().(conversationAttachmentMsg))
	if m.pendingConfirm == nil {
		t.Fatal("missing first conflict")
	}
	staleYes, staleNo := m.pendingConfirm.onYes, m.pendingConfirm.onNo
	m, cmd = m.resolveConfirmKey("enter")
	if cmd != nil || peer.takeovers.Load() != 0 {
		t.Fatal("default-no took over")
	}
	if m.ensureTaskSubscription() != nil {
		t.Fatal("decline implicitly retried")
	}
	m, _ = m.beginProgressiveResume("synthetic")
	cmd = m.ensureTaskSubscription()
	defer m.taskSubscriptionCancel()
	if cmd == nil {
		t.Fatal("explicit resume did not retry")
	}
	if _, staleCmd := staleYes(m); staleCmd != nil {
		t.Fatal("old confirmation bypassed generation fence")
	}
	if stale, _ := staleNo(m); stale.taskAttachmentDeclined {
		t.Fatal("old decline marked the new attachment declined")
	}
	m.applyConversationAttachment(cmd().(conversationAttachmentMsg))
	if m.pendingConfirm == nil || peer.attempts.Load() != 2 || peer.takeovers.Load() != 0 {
		t.Fatal("retry did not use normal attachment before confirmation")
	}
	m, cmd = m.resolveConfirmKey("y")
	if cmd == nil {
		t.Fatal("explicit yes missing takeover")
	}
	ack, ok := cmd().(conversationAttachmentMsg)
	if !ok || !ack.attached || peer.takeovers.Load() != 1 {
		t.Fatalf("fresh-target takeover failed: %+v", ack)
	}
	m, _ = m.beginProgressiveResume("synthetic")
	if m.ensureTaskSubscription() != nil {
		t.Fatal("successful attachment was unnecessarily restarted")
	}
}
