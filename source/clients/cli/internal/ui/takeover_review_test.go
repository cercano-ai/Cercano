package ui

import (
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestInheritedForegroundOutputAndPermission(t *testing.T) {
	m := New(nil, false)
	m.convID = "owner"
	m.ensureTaskSubscription()
	deliver := func(seq uint64, message agentclient.StreamMsg) {
		m.applyConversationAttachment(conversationAttachmentMsg{parent: "owner", generation: m.taskSubscriptionGeneration, update: agentclient.ConversationUpdate{Response: &proto.StreamProcessResponse{TurnEpoch: "host", TurnGeneration: 1, EventSequence: seq}, Message: &message}})
	}
	deliver(1, agentclient.StreamMsg{Type: agentclient.TypeToken, Token: "inherited output"})
	found := false
	for _, e := range m.mainChat().entries {
		if e.Content == "inherited output" {
			found = true
		}
	}
	if !found {
		t.Error("inherited foreground output dropped")
	}
	deliver(0, agentclient.StreamMsg{Type: agentclient.TypePermissionRequired, ToolUseID: "pending", ToolName: "Edit", Tier: "W", ArgsJSON: "{}"})
	if m.pendingConfirm == nil {
		t.Error("inherited pending permission dropped")
	}
}

func TestEnrollmentBeforeTaskSubscription(t *testing.T) {
	m := New(&agentclient.Client{}, false)
	m.convID = "owner"
	cmd := m.ensureTaskSubscription()
	m.taskSubscriptionCancel()
	// A batch starts both network calls concurrently, before enrollment can ack.
	if _, concurrent := cmd().(tea.BatchMsg); concurrent {
		t.Fatal("task subscription starts concurrently with enrollment")
	}
}

// A pinned, loopback-only transport peer that rejects task panes until the
// attachment acknowledgement. No model, credentials, or retained store.
type enrollmentPeer struct {
	closeSessionPeer
	enrolled atomic.Bool
	tasks    atomic.Int32
	expected string
}

func (p *enrollmentPeer) AttachConversation(_ *proto.AttachConversationRequest, stream proto.Agent_AttachConversationServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	if p.expected != "" && (len(md.Get(agentclient.TerminalTakeoverMetadata)) != 1 || md.Get(agentclient.TerminalTakeoverMetadata)[0] != p.expected) {
		return status.Error(codes.PermissionDenied, "explicit takeover required")
	}
	p.enrolled.Store(true)
	if err := stream.SendHeader(metadata.Pairs("cercano-terminal-attached", "true")); err != nil {
		return err
	}
	// Idle conversations still enroll: task observation must not wait for output.
	<-stream.Context().Done()
	return stream.Context().Err()
}
func (p *enrollmentPeer) SubscribeTasks(_ *proto.SubscribeTasksRequest, stream proto.Agent_SubscribeTasksServer) error {
	p.tasks.Add(1)
	if !p.enrolled.Load() {
		return status.Error(codes.PermissionDenied, "terminal not attached")
	}
	if err := stream.Send(&proto.TaskSubscriptionEvent{}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}
func TestTaskObservationAfterEnrollmentTransport(t *testing.T) {
	for _, expected := range []string{"", "previous-terminal"} {
		t.Run("takeover="+expected, func(t *testing.T) {
			p := &enrollmentPeer{expected: expected}
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			srv := grpc.NewServer()
			proto.RegisterAgentServer(srv, p)
			go srv.Serve(l)
			defer srv.Stop()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			c, err := agentclient.DialPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: l.Addr().String(), HostInstance: "test", Build: "build", Contract: "contract", SchemaVersion: 2, ConversationID: "synthetic", OwnershipEpoch: "epoch"})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			// Reproduce the SDK terminal error for an unenrolled task pane.
			early, err := c.SubscribeTasks(ctx, "synthetic")
			if err != nil {
				t.Fatal(err)
			}
			select {
			case update := <-early:
				if status.Code(update.Err) != codes.PermissionDenied {
					t.Fatalf("unenrolled task pane: %+v", update)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			select {
			case _, open := <-early:
				if open {
					t.Fatal("failed task observer did not end")
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			p.tasks.Store(0)
			m := New(c, false)
			m.convID = "synthetic"
			cmd := m.ensureTaskSubscription()
			defer m.taskSubscriptionCancel()
			if expected != "" {
				cmd = attachMainConversationWithTakeoverCmd(m.taskSubscriptionContext, c, m.convID, m.taskSubscriptionGeneration, expected)
			}
			ack, ok := cmd().(conversationAttachmentMsg)
			if !ok || !ack.attached {
				t.Fatalf("no enrollment ack: %+v", ack)
			}
			if p.tasks.Load() != 0 {
				t.Fatal("task observer ran before attachment completion")
			}
			stale := ack
			stale.generation++
			if m.applyConversationAttachment(stale) != nil {
				t.Fatal("stale enrollment started task observer")
			}
			batch := m.applyConversationAttachment(ack)().(tea.BatchMsg)
			if len(batch) != 2 {
				t.Fatalf("commands=%d", len(batch))
			}
			// First command awaits main output; second must start tasks even while idle.
			result := batch[1]().(taskSubscriptionMsg)
			if result.update.Err != nil || result.update.Event == nil || p.tasks.Load() != 1 {
				t.Fatalf("task observer failed: %+v", result.update)
			}
			m.taskSubscriptionCancel()
			if subscribeTasksCmd(m.taskSubscriptionContext, c, m.convID, m.taskSubscriptionGeneration)() != nil {
				t.Fatal("canceled subscription restarted")
			}
		})
	}
}
