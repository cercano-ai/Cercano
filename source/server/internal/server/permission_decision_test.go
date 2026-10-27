package server

import (
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/config"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/hostsvc/permissions"
	"cercano/source/server/pkg/proto"
)

// Reproduces the compiled client's RPC reaching the blanket prepared refusal,
// even though an operator-visible decision barrier already exists.
func TestPreparedPermissionBlanketRefusalReproduction(t *testing.T) {
	s := &Server{coexistence: &coexistenceLifecycle{}, permBroker: permissions.New(nil, agent.NewPendingDecisions(), nil)}
	rpc := permissionRPC(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := s.permBroker.WaitWithNotify(ctx, "c", "tool", func() error {
		_, err := rpc.AllowToolCall(ctx, &proto.AllowToolCallRequest{ConversationId: "c", ToolUseId: "tool"})
		if err == nil {
			t.Fatal("missing binding accepted")
		}
		t.Logf("actual AllowToolCall RPC refusal: %v", err)
		cancel()
		return nil
	})
	if err == nil {
		t.Fatal("wait unexpectedly resolved")
	}
}

func permissionFixture(t *testing.T) *Server {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.db")
	if err := conversation.PrepareCoexistence(ctx, path); err != nil {
		t.Fatal(err)
	}
	s, err := NewPreparedServer(ctx, PreparedServerOptions{
		Coexistence: CoexistenceOptions{StorePath: path, Host: "h", Build: "build", Contract: conversation.CurrentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }},
		Config:      config.Config{OpenRuntime: "synthetic"}, ConfigPath: filepath.Join(dir, "config"), RoutingPath: filepath.Join(dir, "routing"), FailurePath: filepath.Join(dir, "failures"),
		Agent: func(st conversation.Store) (*agent.Agent, error) {
			return agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil
		},
	}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.closeTaskManager()
		for _, o := range s.coexistence.owners {
			_ = o.Close()
		}
		_ = s.coexistence.store.Close()
		if s.visionStore != nil {
			_ = s.visionStore.Close()
		}
	})

	return s
}

func TestPreparedPermissionDecisions(t *testing.T) {
	for _, allow := range []bool{true, false} {
		t.Run(map[bool]string{true: "allow", false: "deny"}[allow], func(t *testing.T) {
			s := permissionFixture(t)
			rpc := permissionRPC(t, s)
			owner, err := s.Retain(context.Background(), "c")
			if err != nil {
				t.Fatal(err)
			}
			binding := func(o broker.Owner, terminal string) context.Context {
				md := metadata.Pairs("cercano-host-instance", o.Host, "cercano-conversation-id", o.Conversation, "cercano-ownership-epoch", o.Epoch, "cercano-terminal-attachment", terminal)
				return metadata.NewOutgoingContext(metadata.NewIncomingContext(context.Background(), md), md)
			}
			exact := binding(owner, "terminal")
			workctx, attachment, ticket, err := s.BeginTerminalStream(exact, "c", broker.ForegroundWork)
			if err != nil {
				t.Fatal(err)
			}
			defer attachment.Detach()
			defer ticket.Complete()
			s.permBroker = permissions.New(nil, agent.NewPendingDecisions(), nil)
			ready := make(chan struct{})
			done := make(chan agent.Decision, 1)
			ctx, cancel := context.WithTimeout(workctx, 5*time.Second)
			defer cancel()
			go func() {
				d, _ := s.waitPermission(ctx, "c", "tool", func() error { close(ready); return nil })
				done <- d
			}()
			<-ready
			// A second turn cannot replace the registered barrier under its key.
			if _, err := s.waitPermission(ctx, "c", "tool", func() error { t.Fatal("duplicate prompt published"); return nil }); err == nil {
				t.Fatal("duplicate gate accepted")
			}
			// Even exact current metadata must not resolve a retained old-epoch gate.
			s.permissionMu.Lock()
			gate := s.permissionGates[permissionGateKey{"c", "tool"}]
			gate.owner.Epoch = "old"
			s.permissionMu.Unlock()
			if _, err := rpc.AllowToolCall(exact, &proto.AllowToolCallRequest{ConversationId: "c", ToolUseId: "tool"}); err == nil {
				t.Fatal("old epoch gate resolved")
			}
			s.permissionMu.Lock()
			gate.owner = owner
			s.permissionMu.Unlock()
			other, err := s.Retain(context.Background(), "other-conversation")
			if err != nil {
				t.Fatal(err)
			}
			otherctx := binding(other, "other-terminal")
			_, otherAttachment, _, err := s.BeginTerminalStream(otherctx, other.Conversation, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer otherAttachment.Detach()
			if _, err := rpc.AllowToolCall(otherctx, &proto.AllowToolCallRequest{ConversationId: other.Conversation, ToolUseId: "tool"}); err == nil {
				t.Fatal("foreign conversation stole decision")
			}
			old := owner
			old.Epoch = "old"
			foreign := owner
			foreign.Host = "other"
			for _, bad := range []context.Context{context.Background(), binding(old, "terminal"), binding(foreign, "terminal"), binding(owner, "never-attached")} {
				if _, err := rpc.AllowToolCall(bad, &proto.AllowToolCallRequest{ConversationId: "c", ToolUseId: "tool"}); err == nil {
					t.Fatal("invalid binding accepted")
				}
			}
			for _, req := range []*proto.AllowToolCallRequest{
				{ConversationId: "c", ToolUseId: "tool", Persist: true},
				{ConversationId: "c", ToolUseId: "unknown"},
				{ConversationId: "other", ToolUseId: "tool"},
			} {
				if _, err := rpc.AllowToolCall(exact, req); err == nil {
					t.Fatal("invalid decision accepted", req)
				}
			}
			select {
			case <-done:
				t.Fatal("refusal resolved pending gate")
			default:
			}
			if allow {
				r, e := rpc.AllowToolCall(exact, &proto.AllowToolCallRequest{ConversationId: "c", ToolUseId: "tool"})
				if e != nil || !r.GetOk() {
					t.Fatal(r, e)
				}
			} else {
				r, e := rpc.DenyToolCall(exact, &proto.DenyToolCallRequest{ConversationId: "c", ToolUseId: "tool", Message: "redirect"})
				if e != nil || !r.GetOk() {
					t.Fatal(r, e)
				}
			}
			select {
			case d := <-done:
				if d.Allow != allow || d.Persist {
					t.Fatal(d)
				}
				if !allow && d.Message != "redirect" {
					t.Fatal(d)
				}
			case <-ctx.Done():
				t.Fatal("admitted turn did not continue")
			}
			if ctx.Err() != nil {
				t.Fatal("decision canceled the admitted turn")
			}
			if err := ticket.Advance(broker.PersistenceWork); err != nil {
				t.Fatalf("original admitted work cannot continue: %v", err)
			}
			if _, err := rpc.AllowToolCall(exact, &proto.AllowToolCallRequest{ConversationId: "c", ToolUseId: "tool"}); err == nil {
				t.Fatal("replayed decision accepted")
			}
		})
	}
}

func permissionRPC(t *testing.T, s *Server) proto.AgentClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	proto.RegisterAgentServer(srv, s)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///permission-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return proto.NewAgentClient(conn)
}
