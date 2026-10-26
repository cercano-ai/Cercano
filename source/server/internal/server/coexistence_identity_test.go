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
	// The lifecycle/admission integration owner supplies this leaf-lock accessor.
	// Until then the handler is explicitly unavailable, never an enrolling probe.
	if _, ok := any(a.coexistence.admission).(retainedIdentityReader); !ok {
		if _, err := a.GetDevelopmentHostIdentity(context.Background(), nil); status.Code(err) != codes.FailedPrecondition {
			t.Fatal(err)
		}
		t.Skip("requires broker.Admission.CheckRetained read-only accessor from lifecycle integration")
	}
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

// Required admission-side implementation (outside this sub-agent's ownership):
//
//	func (a *Admission) CheckRetained(o Owner) (retired, retained, open bool) {
//	  a.mu.Lock(); defer a.mu.Unlock()
//	  s := a.sessionLocked(o)
//	  return a.retired, s != nil, !a.retired && s != nil && !s.closed
//	}
