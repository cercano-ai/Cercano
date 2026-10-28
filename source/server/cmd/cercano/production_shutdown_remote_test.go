package main

import (
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestProductionRemoteStatusDuringBlockedCallback(t *testing.T) {
	gate := newProductionAdmissionGate()
	listener := bufconn.Listen(1 << 20)
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := devhostauth.Create(tokenPath); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load("isolated-control", tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer(append([]grpc.ServerOption{grpc.ChainUnaryInterceptor(gate.unary)}, auth.ServerOptions()...)...)
	srv := &server.Server{}
	release := make(chan struct{})
	shutdown := newProductionShutdown(func() { gate.close(); srv.FenceBackgroundWork() }, func() error { <-release; return nil })
	srv.SetShutdownControl(shutdown.Control)
	proto.RegisterAgentServer(gs, srv)
	go gs.Serve(listener)
	defer gs.GracefulStop()
	dial := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() })}
	conn, err := grpc.NewClient("passthrough:///probe", append(dial, auth.DialOptions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer func() { close(release); shutdown.Request(); <-shutdown.done }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	before, err := proto.NewAgentClient(conn).ShutdownAgent(ctx, &proto.ShutdownAgentRequest{StatusOnly: true})
	if err != nil || before.Accepted || before.Complete || before.Phase != "running" {
		t.Fatalf("observation initiated shutdown: %v %v", before, err)
	}
	first, err := proto.NewAgentClient(conn).ShutdownAgent(ctx, &proto.ShutdownAgentRequest{Reason: "secret-do-not-echo"})
	if err != nil || !first.Accepted || first.Complete {
		t.Fatalf("request: %v %v", first, err)
	}
	if first.IncompleteReason != "callbacks or resource disposal pending" {
		t.Fatal("nonconstant reason")
	}
	unauth, err := grpc.NewClient("passthrough:///probe", dial...)
	if err != nil {
		t.Fatal(err)
	}
	defer unauth.Close()
	_, err = proto.NewAgentClient(unauth).ShutdownAgent(ctx, &proto.ShutdownAgentRequest{StatusOnly: true})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("closed gate bypassed authentication: %v", err)
	}
	result, err := proto.NewAgentClient(conn).ShutdownAgent(ctx, &proto.ShutdownAgentRequest{StatusOnly: true})
	if err != nil {
		t.Fatalf("status unavailable during blocked callback: %v", err)
	}
	if result.Complete || !result.Accepted || result.Phase != "draining" || result.IncompleteReason == "" {
		t.Fatal(result)
	}
	_, err = proto.NewAgentClient(conn).ListConversations(ctx, &proto.ListConversationsRequest{})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("ordinary work admitted: %v", err)
	}
}
