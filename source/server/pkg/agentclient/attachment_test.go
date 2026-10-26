package agentclient

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type backpressuredAttachmentServer struct {
	proto.UnimplementedAgentServer
	stopped chan struct{}
}

func (s *backpressuredAttachmentServer) AttachConversation(_ *proto.AttachConversationRequest, stream proto.Agent_AttachConversationServer) error {
	defer close(s.stopped)
	for seq := uint64(1); seq <= 100; seq++ {
		if err := stream.Send(&proto.StreamProcessResponse{TurnEpoch: "host", TurnGeneration: 1, EventSequence: seq, BackgroundTurn: true, Payload: &proto.StreamProcessResponse_TokenDelta{TokenDelta: &proto.TokenDelta{Content: "x"}}}); err != nil {
			return err
		}
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}
func TestAttachmentCancellationReleasesBackpressuredObserver(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &backpressuredAttachmentServer{stopped: make(chan struct{})}
	server := grpc.NewServer()
	proto.RegisterAgentServer(server, service)
	go server.Serve(listener)
	defer server.Stop()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, err := DialExisting(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	updates, err := client.AttachConversation(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	for len(updates) < cap(updates) {
		select {
		case <-ctx.Done():
			t.Fatal("observer never filled bounded queue")
		default:
			runtime.Gosched()
		}
	}
	cancel()
	select {
	case <-service.stopped:
	case <-time.After(time.Second):
		t.Fatal("canceled observer retained its server stream")
	}
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-updates:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("SDK observer did not exit")
		}
	}
}

type reconnectAttachmentServer struct {
	proto.UnimplementedAgentServer
	calls    atomic.Int32
	requests chan *proto.AttachConversationRequest
}

func (s *reconnectAttachmentServer) AttachConversation(req *proto.AttachConversationRequest, stream proto.Agent_AttachConversationServer) error {
	call := s.calls.Add(1)
	s.requests <- req
	if err := stream.Send(&proto.StreamProcessResponse{TurnEpoch: "epoch", TurnGeneration: 7, EventSequence: uint64(call), BackgroundTurn: true, Payload: &proto.StreamProcessResponse_TokenDelta{TokenDelta: &proto.TokenDelta{Content: "reply"}}}); err != nil {
		return err
	}
	if call == 1 {
		return status.Error(codes.Unavailable, "reconnect probe")
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}
func TestConversationAttachmentReconnectCarriesCursor(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &reconnectAttachmentServer{requests: make(chan *proto.AttachConversationRequest, 2)}
	server := grpc.NewServer()
	proto.RegisterAgentServer(server, service)
	go server.Serve(listener)
	defer server.Stop()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, err := DialExisting(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	updates, err := client.AttachConversation(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	for want := uint64(1); want <= 2; want++ {
		select {
		case update, ok := <-updates:
			if !ok || update.Err != nil || update.Response.GetEventSequence() != want {
				t.Fatalf("update=%+v want=%d", update, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	<-service.requests
	resumed := <-service.requests
	if resumed.ConversationId != "owner" || resumed.AfterTurnEpoch != "epoch" || resumed.AfterTurnGeneration != 7 || resumed.AfterEventSequence != 1 {
		t.Fatalf("resume request=%v", resumed)
	}
	cancel()
	for range updates {
	}
}

func (s backpressuredAttachmentServer) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}

func (s reconnectAttachmentServer) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
