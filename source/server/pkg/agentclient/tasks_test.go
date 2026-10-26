package agentclient

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type reconnectTaskServer struct {
	proto.UnimplementedAgentServer
	calls    atomic.Int32
	requests chan *proto.SubscribeTasksRequest
	canceled chan struct{}
}

func (s *reconnectTaskServer) SubscribeTasks(req *proto.SubscribeTasksRequest, stream proto.Agent_SubscribeTasksServer) error {
	call := s.calls.Add(1)
	s.requests <- req
	sequence := int64(call)
	if err := stream.Send(&proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Progress{Progress: &proto.TaskProgressRecord{TaskId: "task", ChildId: "child", Sequence: sequence, Kind: "note", PayloadJson: `{}`}}}); err != nil {
		return err
	}
	if call == 1 {
		return status.Error(codes.Unavailable, "reconnect probe")
	}
	<-stream.Context().Done()
	close(s.canceled)
	return stream.Context().Err()
}
func TestSubscribeTasksReopensWithDeliveredCursor(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &reconnectTaskServer{requests: make(chan *proto.SubscribeTasksRequest, 2), canceled: make(chan struct{})}
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
	updates, err := client.SubscribeTasks(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	for want := int64(1); want <= 2; want++ {
		select {
		case update := <-updates:
			if update.Err != nil || update.Event.GetProgress().GetSequence() != want {
				t.Fatalf("update=%+v want sequence=%d", update, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	first, second := <-service.requests, <-service.requests
	if first.ParentId != "owner" || second.ParentId != "owner" || len(second.Cursors) != 1 || second.Cursors[0].TaskId != "task" || second.Cursors[0].Sequence != 1 {
		t.Fatalf("reconnect cursor: %v", second)
	}
	cancel()
	select {
	case <-service.canceled:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not close observation")
	}
	for range updates {
	}
	if client.currentState() == ConnStateFailed {
		t.Fatal("observer cancellation poisoned shared connection state")
	}
}

func (s reconnectTaskServer) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
