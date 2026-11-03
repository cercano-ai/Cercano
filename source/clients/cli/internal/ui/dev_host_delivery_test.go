package ui

import (
	"context"
	"net"
	"path/filepath"
	"testing"

	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type deliveryHost struct {
	pb.UnimplementedAgentServer
	ambiguous bool
	called    bool
}

func (h *deliveryHost) LifecycleAcquire(_ context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	h.called = true
	if r.LaunchAction != "deliver" || r.LaunchOperation != "operation" || r.Owner.Epoch != "epoch" {
		return &pb.LifecycleControlResponse{}, nil
	}
	return &pb.LifecycleControlResponse{Identity: r.Target, Owner: r.Owner, Receipt: r.LaunchOperation, Pending: h.ambiguous}, nil
}
func TestDevHostLaunchDeliveryBoundary(t *testing.T) {
	token := filepath.Join(t.TempDir(), "token")
	if err := devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := &deliveryHost{}
	server := grpc.NewServer()
	pb.RegisterAgentServer(server, host)
	go server.Serve(listener)
	defer server.Stop()
	s := DevHostSelection{LaunchOperation: "operation", OperationFile: "retry.json", Identity: &pb.LifecycleIdentity{Host: "host"}, Conversation: "conversation", Epoch: "epoch", Endpoint: listener.Addr().String(), TokenFile: token}
	if err := s.AcceptLaunchDelivery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !host.called {
		t.Fatal("delivery skipped")
	}
	// Server replies are serialized by this RPC; change the fixture only after return.
	server.Stop()
	// Incomplete new selections must fail before opening a usable client.
	s.Epoch = ""
	if _, err := s.Dial(context.Background()); err == nil {
		t.Fatal("incomplete receipt accepted")
	}
	if err := (DevHostSelection{}).AcceptLaunchDelivery(context.Background()); err != nil {
		t.Fatal("legacy selection changed", err)
	}
}
