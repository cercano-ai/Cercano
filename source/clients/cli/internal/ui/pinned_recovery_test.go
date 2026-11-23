package ui

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type recoveryHost struct {
	proto.UnimplementedAgentServer
	checks, requests atomic.Int32
}

func (*recoveryHost) GetDevelopmentHostIdentity(context.Context, *proto.GetDevelopmentHostIdentityRequest) (*proto.GetDevelopmentHostIdentityResponse, error) {
	return &proto.GetDevelopmentHostIdentityResponse{HostInstance: "A", Contract: "contract", SchemaVersion: 2}, nil
}
func (h *recoveryHost) CheckRetainedSession(context.Context, *proto.CheckRetainedSessionRequest) (*proto.CheckRetainedSessionResponse, error) {
	h.checks.Add(1)
	return &proto.CheckRetainedSessionResponse{State: proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED}, nil
}
func (h *recoveryHost) StreamProcessRequest(*proto.ProcessRequestRequest, grpc.ServerStreamingServer[proto.StreamProcessResponse]) error {
	h.requests.Add(1)
	return status.Error(codes.Unavailable, "ambiguous delivery")
}

func TestMainDriverPinnedRecoveryDoesNotResubmit(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h := &recoveryHost{}
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
	msg, ok := wait().(chatStreamMsg)
	if !ok {
		t.Fatal("missing error event")
	}
	// The driver must preserve the recoverable SDK error, not start another turn.
	failure, ok := msg.ev.(chatErrorMsg)
	if !ok || !strings.Contains(failure.err.Error(), "not resent") {
		t.Fatalf("missing delivery warning: %v", msg.ev)
	}
	if h.requests.Load() != 1 || h.checks.Load() != 2 {
		t.Fatalf("requests=%d validations=%d", h.requests.Load(), h.checks.Load())
	}
}
