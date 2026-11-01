package agentclient

import (
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"net"
	"testing"
	"time"
)

type takeoverFake struct {
	pinnedFake
	attempts chan metadata.MD
}

func (f *takeoverFake) AttachConversation(_ *proto.AttachConversationRequest, stream proto.Agent_AttachConversationServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	f.attempts <- md
	if err := stream.SendHeader(metadata.Pairs("cercano-terminal-attached", "true")); err != nil {
		return err
	}
	if len(md.Get(TerminalTakeoverMetadata)) != 0 {
		return status.Error(codes.Unavailable, "retry")
	}
	return status.Error(codes.PermissionDenied, "revoked")
}
func TestTakeoverMetadataOnlyOnInitialEnrollment(t *testing.T) {
	f := &takeoverFake{pinnedFake: pinnedFake{host: "A"}, attempts: make(chan metadata.MD, 4)}
	f.state.Store(1)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	proto.RegisterAgentServer(srv, f)
	go srv.Serve(l)
	defer srv.Stop()
	intent := pinAt(l.Addr().String())
	intent.Build = "different-client-build"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := DialPinnedSession(ctx, intent)
	if err != nil {
		t.Fatalf("build should be diagnostic: %v", err)
	}
	defer c.Close()
	updates, err := c.TakeoverConversation(ctx, "conversation", "old")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-updates:
		if status.Code(update.Err) != codes.PermissionDenied {
			t.Fatalf("unexpected result: %v", update.Err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	first, second := <-f.attempts, <-f.attempts
	if first.Get(TerminalTakeoverMetadata)[0] != "old" || len(second.Get(TerminalTakeoverMetadata)) != 0 {
		t.Fatal("takeover repeated on retry")
	}
	if first.Get(PinnedTerminalMetadata)[0] != second.Get(PinnedTerminalMetadata)[0] {
		t.Fatal("retry changed identity")
	}
}
