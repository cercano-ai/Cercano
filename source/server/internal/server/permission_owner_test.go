package server

import (
	"context"
	"testing"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/hostsvc/permissions"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/metadata"
)

func TestPermissionReleasedAndRetiredOwner(t *testing.T) {
	for _, retire := range []bool{false, true} {
		s := permissionFixture(t)
		owner, err := s.Retain(context.Background(), "c")
		if err != nil {
			t.Fatal(err)
		}
		md := metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "terminal")
		ctx := metadata.NewIncomingContext(context.Background(), md)
		_, a, _, err := s.BeginTerminalStream(ctx, "c", 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ReleaseRetained(context.Background(), owner, a); err != nil {
			t.Fatal(err)
		}
		a.Detach()
		if retire {
			if err := s.coexistence.admission.Retire(); err != nil {
				t.Fatal(err)
			}
		}
		s.permBroker = permissions.New(nil, agent.NewPendingDecisions(), nil)
		waitctx, cancel := context.WithCancel(context.Background())
		ready := make(chan struct{})
		done := make(chan agent.Decision, 1)
		s.permissionGates = map[permissionGateKey]*permissionGate{{"c", "tool"}: {owner: owner, ctx: waitctx}}
		go func() {
			d, _ := s.permBroker.WaitWithNotify(waitctx, "c", "tool", func() error { close(ready); return nil })
			done <- d
		}()
		<-ready
		rpc := permissionRPC(t, s)
		outgoing := metadata.NewOutgoingContext(context.Background(), md)
		if _, err := rpc.AllowToolCall(outgoing, &proto.AllowToolCallRequest{ConversationId: "c", ToolUseId: "tool"}); err == nil {
			t.Fatal("released owner approved")
		}
		if _, err := rpc.DenyToolCall(outgoing, &proto.DenyToolCallRequest{ConversationId: "c", ToolUseId: "tool"}); err == nil {
			t.Fatal("released owner denied")
		}
		select {
		case <-done:
			t.Fatal("refusal changed pending decision")
		default:
		}
		cancel()
		<-done
	}
}

func TestLegacyPermissionPersistUnchanged(t *testing.T) {
	s := &Server{permBroker: permissions.New(nil, agent.NewPendingDecisions(), nil)}
	rpc := permissionRPC(t, s)
	d, err := s.waitPermission(context.Background(), "c", "tool", func() error {
		r, e := rpc.AllowToolCall(context.Background(), &proto.AllowToolCallRequest{ConversationId: "c", ToolUseId: "tool", Persist: true})
		if e != nil {
			return e
		}
		if !r.GetOk() {
			t.Fatal("legacy approval refused")
		}
		return nil
	})
	if err != nil || !d.Allow || !d.Persist {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}
