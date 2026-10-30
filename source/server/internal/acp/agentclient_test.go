package acp

import (
	"context"
	"fmt"
	"strings"

	"net"
	"testing"
	"time"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type grpcBackend struct {
	proto.UnimplementedAgentServer
	requests  chan *proto.ProcessRequestRequest
	decisions chan decision
	approved  chan bool
	attached  chan string
	cancelled chan string
}

func (s *grpcBackend) AttachConversation(req *proto.AttachConversationRequest, stream proto.Agent_AttachConversationServer) error {
	s.attached <- req.ConversationId
	<-stream.Context().Done()
	return stream.Context().Err()
}
func (s *grpcBackend) StreamProcessRequest(req *proto.ProcessRequestRequest, stream proto.Agent_StreamProcessRequestServer) error {
	s.requests <- req
	defer func() { s.cancelled <- req.ConversationId }()
	if err := stream.Send(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_PermissionRequired{PermissionRequired: &proto.PermissionRequired{ToolUseId: "grpc-tool", ToolName: "Write", ArgsJson: `{"path":"x"}`}}}); err != nil {
		return err
	}
	select {
	case <-stream.Context().Done():
		return stream.Context().Err()
	case <-s.approved:
	}
	return stream.Send(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_FinalResponse{FinalResponse: &proto.ProcessRequestResponse{Output: "done"}}})
}
func (s *grpcBackend) AllowToolCall(_ context.Context, r *proto.AllowToolCallRequest) (*proto.AllowToolCallResponse, error) {
	s.decisions <- decision{r.ConversationId, r.ToolUseId, true}
	s.approved <- true
	return &proto.AllowToolCallResponse{}, nil
}
func (s *grpcBackend) DenyToolCall(_ context.Context, r *proto.DenyToolCallRequest) (*proto.DenyToolCallResponse, error) {
	s.decisions <- decision{r.ConversationId, r.ToolUseId, false}
	return &proto.DenyToolCallResponse{}, nil
}
func TestGRPCAdapterPermissionAndCancel(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "allow", true: "cancel"}[cancel], func(t *testing.T) {
			service := &grpcBackend{requests: make(chan *proto.ProcessRequestRequest, 2), decisions: make(chan decision, 2), approved: make(chan bool, 1), attached: make(chan string, 2), cancelled: make(chan string, 2)}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			gs := grpc.NewServer()
			proto.RegisterAgentServer(gs, service)
			go gs.Serve(listener)
			t.Cleanup(gs.Stop)
			client, err := agentclient.DialExisting(context.Background(), listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			h := start(t, NewAgentBackend(context.Background(), client))
			h.init()
			sid := h.newSession()
			if got := recv(t, service.attached); got != sid {
				t.Fatal(got)
			}
			h.prompt(3, sid)
			req := recv(t, service.requests)
			if req.ConversationId != sid || req.WorkDir != "/workspace" || req.Input != "hello" || req.SupportsAuthRecovery {
				t.Fatal(req)
			}
			permission := h.next()
			if cancel {
				h.send(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]any{"sessionId": sid}})
			} else {
				h.send(permissionReply(permission["id"], "allow_once"))
			}
			d := recv(t, service.decisions)
			if d.sid != sid || d.tool != "grpc-tool" || d.allow == cancel {
				t.Fatal(d)
			}
			if !cancel {
				if f := h.next(); f["method"] != "session/update" {
					t.Fatal(f)
				}
			}
			f := h.next()
			want := "end_turn"
			if cancel {
				want = "cancelled"
			}
			if f["result"].(map[string]any)["stopReason"] != want {
				t.Fatal(f)
			}
			recv(t, service.cancelled)
			h.in.Close()
			if err := recv(t, h.done); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type streamClient struct{ messages chan agentclient.StreamMsg }

func (c *streamClient) StreamChat(context.Context, string, string, string, ...agentclient.InlineImage) (<-chan agentclient.StreamMsg, error) {
	return c.messages, nil
}
func (c *streamClient) AttachConversation(ctx context.Context, _ string) (<-chan agentclient.ConversationUpdate, error) {
	ch := make(chan agentclient.ConversationUpdate)
	return ch, nil
}
func (c *streamClient) AllowToolCall(context.Context, string, string) error { return nil }
func (c *streamClient) DenyToolCall(context.Context, string, string) error  { return nil }
func (c *streamClient) Close() error                                        { return nil }
func TestAdapterCancellationStopsWaitingForStream(t *testing.T) {
	c := &streamClient{messages: make(chan agentclient.StreamMsg)}
	b := NewAgentBackend(context.Background(), c)
	ctx, cancel := context.WithCancel(context.Background())
	out, err := b.Prompt(ctx, "s", "/workspace", "hello")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	defer close(c.messages)
	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("unexpected event")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("adapter stuck awaiting backend stream after cancellation")
	}
	b.Close()
}

func TestAdapterAuthAndRolloverFailActionably(t *testing.T) {
	for _, kind := range []agentclient.StreamMsgType{agentclient.TypeAuthentication, agentclient.TypeRolloverOffered} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			c := &streamClient{messages: make(chan agentclient.StreamMsg, 1)}
			b := NewAgentBackend(context.Background(), c)
			h := start(t, b)
			h.init()
			sid := h.newSession()
			h.prompt(3, sid)
			c.messages <- agentclient.StreamMsg{Type: kind}
			f := h.next()
			e, ok := f["error"].(map[string]any)
			if !ok || !strings.Contains(e["message"].(string), "Cercano CLI") {
				t.Fatal(f)
			}
			h.in.Close()
			if err := recv(t, h.done); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type lifetimeClient struct {
	*streamClient
	subscribed int
}

func (c *lifetimeClient) SubscribeEvents(context.Context) (<-chan agentclient.AgentEvent, error) {
	c.subscribed++
	return make(chan agentclient.AgentEvent), nil
}
func TestAdapterRetainsDaemonLifetime(t *testing.T) {
	c := &lifetimeClient{streamClient: &streamClient{messages: make(chan agentclient.StreamMsg)}}
	b := NewAgentBackend(context.Background(), c)
	defer b.Close()
	for i := 0; i < 2; i++ {
		if _, err := b.NewSession(context.Background(), "/workspace"); err != nil {
			t.Fatal(err)
		}
	}
	if c.subscribed != 1 {
		t.Fatalf("standing daemon subscriptions=%d; want 1 (attachments do not retain daemon)", c.subscribed)
	}
}

func (c *streamClient) SubscribeEvents(context.Context) (<-chan agentclient.AgentEvent, error) {
	return make(chan agentclient.AgentEvent), nil
}
func (s *grpcBackend) SubscribeEvents(_ *proto.SubscribeEventsRequest, stream proto.Agent_SubscribeEventsServer) error {
	<-stream.Context().Done()
	return stream.Context().Err()
}
