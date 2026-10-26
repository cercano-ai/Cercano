package server

import (
	"context"
	"errors"
	"net"
	"testing"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCoexistenceIdentityDisabled(t *testing.T) {
	s := &Server{}
	if _, err := s.GetDevelopmentHostIdentity(context.Background(), nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if _, err := s.CheckRetainedSession(context.Background(), nil); status.Code(err) != codes.InvalidArgument {
		t.Fatal(err)
	}
}

func TestCoexistenceIdentityPinnedIntegration(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	a, _, _ := lifecycleFixture(t)
	ctx := context.Background()
	o, err := a.Retain(ctx, "conversation")
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	proto.RegisterAgentServer(g, a)
	go g.Serve(l)
	defer g.Stop()
	d := a.coexistence.options.Contract
	pin := agentclient.PinnedSessionIntent{Endpoint: l.Addr().String(), HostInstance: o.Host, Build: "build-A", Contract: d.Contract, SchemaVersion: int32(d.SchemaVersion), ConversationID: o.Conversation, OwnershipEpoch: o.Epoch}
	c, err := agentclient.DialPinnedSession(ctx, pin)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i := 0; i < 3; i++ {
		if err := c.ReconnectPinnedSession(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// A management release succeeds only if checks created no attachments/work.
	if err := a.ReleaseRetained(ctx, o, nil); err != nil {
		t.Fatalf("check enrolled client: %v", err)
	}
	var refusal *agentclient.SessionCheckError
	if err := c.ReconnectPinnedSession(ctx); !errors.As(err, &refusal) || refusal.State != proto.RetainedSessionState_RETAINED_SESSION_STATE_RELEASED {
		t.Fatalf("release: %v", err)
	}
	fresh, err := a.Retain(ctx, o.Conversation)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ReconnectPinnedSession(ctx); !errors.As(err, &refusal) || refusal.State != proto.RetainedSessionState_RETAINED_SESSION_STATE_STALE {
		t.Fatalf("stale: %v", err)
	}
	if err := a.ReleaseRetained(ctx, fresh, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.RetireCoexistence(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.ReconnectPinnedSession(ctx); !errors.As(err, &refusal) || refusal.State != proto.RetainedSessionState_RETAINED_SESSION_STATE_RETIRED {
		t.Fatalf("retired: %v", err)
	}
}
