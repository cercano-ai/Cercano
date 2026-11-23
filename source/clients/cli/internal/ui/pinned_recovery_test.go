package ui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/charmbracelet/x/ansi"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type recoveryHost struct {
	proto.UnimplementedAgentServer
	checks, requests                        atomic.Int32
	intermediate, continuation, repairFails bool
}

func (*recoveryHost) GetDevelopmentHostIdentity(context.Context, *proto.GetDevelopmentHostIdentityRequest) (*proto.GetDevelopmentHostIdentityResponse, error) {
	return &proto.GetDevelopmentHostIdentityResponse{HostInstance: "A", Contract: "contract", SchemaVersion: 2}, nil
}
func (h *recoveryHost) CheckRetainedSession(context.Context, *proto.CheckRetainedSessionRequest) (*proto.CheckRetainedSessionResponse, error) {
	if h.checks.Add(1) > 1 && h.repairFails {
		return nil, status.Error(codes.FailedPrecondition, "repair refused")
	}
	return &proto.CheckRetainedSessionResponse{State: proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED}, nil
}
func (h *recoveryHost) StreamProcessRequest(_ *proto.ProcessRequestRequest, stream grpc.ServerStreamingServer[proto.StreamProcessResponse]) error {
	h.requests.Add(1)
	if h.intermediate {
		if err := stream.Send(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_FinalResponse{FinalResponse: &proto.ProcessRequestResponse{Output: "intermediate"}}}); err != nil {
			return err
		}
	}
	if h.continuation {
		if err := stream.Send(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_TokenDelta{TokenDelta: &proto.TokenDelta{Content: "continuation"}}}); err != nil {
			return err
		}
	}
	return status.Error(codes.Unavailable, "ambiguous delivery")
}

func TestMainDriverPinnedRecoveryDoesNotResubmit(t *testing.T) {
	for _, tc := range []struct {
		name                                    string
		intermediate, continuation, repairFails bool
	}{
		{"before-response", false, false, false},
		{"immediate/repaired", true, false, false},
		{"immediate/refused", true, false, true},
		{"continuation/repaired", true, true, false},
		{"continuation/refused", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) { testMainDriverPinnedRecovery(t, tc.intermediate, tc.continuation, tc.repairFails) })
	}
}
func testMainDriverPinnedRecovery(t *testing.T, intermediate, continuation, repairFails bool) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h := &recoveryHost{intermediate: intermediate, continuation: continuation, repairFails: repairFails}
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, h)
	go s.Serve(l)
	defer s.Stop()
	c, err := agentclient.DialPinnedSession(context.Background(), agentclient.PinnedSessionIntent{Endpoint: l.Addr().String(), HostInstance: "A", Contract: "contract", SchemaVersion: 2, ConversationID: "conversation", OwnershipEpoch: "epoch"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	d := &mainAgentDriver{agent: c, convID: "conversation"}
	wait, cancel, err := d.Submit(context.Background(), 1, "once", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	m := New(c, false)
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.turnGen = 1
	m.streaming = true
	m.cancelStream = cancel
	m.mainChat().AppendEntry(&Entry{Role: RoleUser, Content: "once"})
	m.mainChat().AppendEntry(&Entry{Role: RoleAssistant, Streaming: true})
	queue := []tea.Cmd{wait}
	deadline := time.After(3 * time.Second)
	for steps := 0; m.streaming && steps < 30 && len(queue) > 0; steps++ {
		cmd := queue[0]
		queue = queue[1:]
		if cmd == nil {
			continue
		}
		result := make(chan tea.Msg, 1)
		go func() { result <- cmd() }()
		var event tea.Msg
		select {
		case event = <-result:
		case <-deadline:
			t.Fatal("update loop timed out")
		}
		if batch, ok := event.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		next, cmd := m.Update(event)
		m = next.(Model)
		queue = append(queue, cmd)
	}
	if m.streaming {
		t.Fatal("stream did not finish")
	}
	retained := false
	for _, entry := range m.mainChat().Entries() {
		retained = retained || strings.Contains(entry.Content, "not resent")
	}
	if !retained || !strings.Contains(strings.Join(strings.Fields(ansi.Strip(m.View().Content)), " "), "not resent") {
		t.Fatalf("delivery warning not retained/rendered: %s", m.View().Content)
	}
	if h.requests.Load() != 1 || h.checks.Load() != 2 {
		t.Fatalf("requests=%d validations=%d", h.requests.Load(), h.checks.Load())
	}
}
