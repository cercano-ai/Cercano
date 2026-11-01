package agentclient

import (
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"net"
	"testing"
	"time"
)

type localAttachmentPeer struct {
	proto.UnimplementedAgentServer
	started chan struct{}
	release chan struct{}
}

func foregroundResponse() *proto.StreamProcessResponse {
	return &proto.StreamProcessResponse{TurnEpoch: "host", TurnGeneration: 1, EventSequence: 1, Payload: &proto.StreamProcessResponse_TokenDelta{TokenDelta: &proto.TokenDelta{Content: "foreground"}}}
}
func (p *localAttachmentPeer) StreamProcessRequest(_ *proto.ProcessRequestRequest, stream proto.Agent_StreamProcessRequestServer) error {
	close(p.started)
	select {
	case <-p.release:
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
	if err := stream.Send(foregroundResponse()); err != nil {
		return err
	}
	return stream.Send(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_PermissionRequired{PermissionRequired: &proto.PermissionRequired{ToolUseId: "pending", ToolName: "Edit", TurnEpoch: "host", TurnGeneration: 1}}})
}
func (p *localAttachmentPeer) AttachConversation(_ *proto.AttachConversationRequest, stream proto.Agent_AttachConversationServer) error {
	if err := stream.Send(foregroundResponse()); err != nil {
		return err
	}
	if err := stream.Send(&proto.StreamProcessResponse{TurnEpoch: "host", TurnGeneration: 1, Payload: &proto.StreamProcessResponse_PermissionRequired{PermissionRequired: &proto.PermissionRequired{ToolUseId: "pending", ToolName: "Edit", TurnEpoch: "host", TurnGeneration: 1}}}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}
func TestForegroundAttachmentTransportLocalAndInherited(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(map[bool]string{false: "inherited", true: "local"}[local], func(t *testing.T) {
			p := &localAttachmentPeer{started: make(chan struct{}), release: make(chan struct{})}
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
			c, err := DialExisting(ctx, l.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if local {
				if _, err = c.StreamChat(ctx, "owner", "input", ""); err != nil {
					t.Fatal(err)
				}
				select {
				case <-p.started:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			updates, err := c.AttachConversation(ctx, "owner")
			if err != nil {
				t.Fatal(err)
			}
			// Force the observer to win the transport race. It must await the initiator's version.
			if local {
				select {
				case u := <-updates:
					t.Fatalf("classified before initiator version: %+v", u)
				case <-time.After(30 * time.Millisecond):
				}
				close(p.release)
			}
			select {
			case u := <-updates:
				if u.Err != nil || u.Local != local || u.Message == nil || u.Message.Token != "foreground" {
					t.Fatalf("update=%+v", u)
				}
				select {
				case prompt := <-updates:
					if prompt.Local != local || prompt.Message == nil || prompt.Message.Type != TypePermissionRequired || prompt.Message.ToolUseID != "pending" {
						t.Fatalf("pending approval=%+v", prompt)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}
func TestLocalAttachmentWaitCancellationAndFailedRequest(t *testing.T) {
	c := &Client{}
	r := c.beginLocalRequest("owner")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := c.localConversationEvent(ctx, "owner", foregroundResponse()); ok {
		t.Fatal("canceled observer waited")
	}
	c.resolveLocalRequest(r, nil)
	if local, ok := c.localConversationEvent(context.Background(), "owner", foregroundResponse()); !ok || local {
		t.Fatal("failed request claimed inherited turn")
	}
}

func TestLocalAttachmentContinuationAndNestedPermission(t *testing.T) {
	c := &Client{}
	r := c.beginLocalRequest("owner")
	first := foregroundResponse()
	c.resolveLocalRequest(r, first)
	next := foregroundResponse()
	next.TurnGeneration = 2
	result := make(chan bool, 1)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	go func() { local, ok := c.localConversationEvent(ctx, "owner", next); result <- local && ok }()
	select {
	case <-result:
		t.Fatal("continuation classified before initiating stream")
	case <-time.After(20 * time.Millisecond):
	}
	c.resolveLocalRequest(r, &proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_PermissionRequired{PermissionRequired: &proto.PermissionRequired{TurnEpoch: "host", TurnGeneration: 2}}})
	select {
	case local := <-result:
		if !local {
			t.Fatal("nested permission not local")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	c.resolveLocalRequest(r, nil)
	if local, ok := c.localConversationEvent(ctx, "owner", first); !ok || !local {
		t.Fatal("completed request replay duplicated")
	}
	next.TurnGeneration = 3
	if local, ok := c.localConversationEvent(ctx, "owner", next); !ok || local {
		t.Fatal("inherited successor suppressed")
	}
}
