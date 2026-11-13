package server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

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

// List owns this same mutex while observing receipts and querying the database.
func TestIdentityIndependentOfLifecycleMutex(t *testing.T) {
	a, _, _ := lifecycleFixture(t)
	a.coexistence.mu.Lock()
	done := make(chan error, 1)
	go func() { _, err := a.GetDevelopmentHostIdentity(context.Background(), nil); done <- err }()
	select {
	case err := <-done:
		a.coexistence.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(100 * time.Millisecond):
		a.coexistence.mu.Unlock()
		<-done
		t.Fatal("production identity blocked behind lifecycle/List mutex")
	}
}

// A canceled List can still acquire the mutex and consume a receipt. This is
// why ordinary authentication must not issue List, even synchronously over RPC.
func TestIdentityWhileActualListQueued(t *testing.T) {
	a, ctx, request := launchFixture(t)
	acquireLaunch(t, a, ctx, request)
	l := a.coexistence
	l.mu.Lock()
	listCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := a.LifecycleList(listCtx, request); done <- err }()
	cancel()
	identity := make(chan error, 1)
	go func() { _, err := a.GetDevelopmentHostIdentity(ctx, nil); identity <- err }()
	select {
	case err := <-identity:
		if err != nil {
			l.mu.Unlock()
			<-done
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		l.mu.Unlock()
		<-done
		<-identity
		t.Fatal("identity queued behind actual List's lifecycle mutex")
	}
	l.mu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := l.launches[request.LaunchOperation].LaunchAction; got != "delivered" {
		t.Fatalf("late List receipt=%s", got)
	}
}
