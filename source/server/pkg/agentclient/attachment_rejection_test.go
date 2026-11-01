package agentclient

import (
	"context"
	"net"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type attachmentRejectionFake struct{ pinnedFake }

func (f *attachmentRejectionFake) AttachConversation(_ *proto.AttachConversationRequest, _ proto.Agent_AttachConversationServer) error {
	rejection, err := status.New(codes.AlreadyExists, "already attached").WithDetails(&errdetails.ErrorInfo{Reason: TerminalConflictReason, Metadata: map[string]string{"terminal": "original-terminal"}})
	if err != nil {
		return err
	}
	// Deliberately no SendHeader: real admission rejects before enrollment.
	return rejection.Err()
}

func TestAttachmentTrailersOnlyConflictPreservesTakeoverTarget(t *testing.T) {
	f := &attachmentRejectionFake{pinnedFake: pinnedFake{host: "A"}}
	f.state.Store(1)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	proto.RegisterAgentServer(srv, f)
	go srv.Serve(listener)
	defer srv.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := DialPinnedSession(ctx, pinAt(listener.Addr().String()))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, takeover := range []bool{false, true} {
		var updates <-chan ConversationUpdate
		if takeover {
			updates, err = client.TakeoverConversation(ctx, "conversation", "obsolete-terminal")
		} else {
			updates, err = client.AttachConversation(ctx, "conversation")
		}
		if updates != nil || TakeoverTarget(err) != "original-terminal" {
			t.Fatalf("takeover=%v: updates=%v error=%v", takeover, updates, err)
		}
	}
}
