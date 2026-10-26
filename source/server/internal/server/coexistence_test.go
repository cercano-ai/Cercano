package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
)

func lifecycleFixture(t *testing.T) (*Server, *Server, *Server) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "owned.db")
	if e := conversation.PrepareCoexistence(ctx, path); e != nil {
		t.Fatal(e)
	}
	makeServer := func(host string) *Server {
		s, e := NewCoexistenceServer(ctx, CoexistenceOptions{StorePath: path, Build: "build-" + host, Host: host, Contract: conversation.CurrentCoexistenceDeclaration(), CloseParticipation: func(context.Context) error { return nil }, Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			for _, o := range s.coexistence.owners {
				_ = o.Close()
			}
			_ = s.coexistence.store.Close()
		})
		return s
	}
	return makeServer("A"), makeServer("B"), makeServer("C")
}
func TestCoexistenceLifecycleHandover(t *testing.T) {
	ctx := context.Background()
	a, b, c := lifecycleFixture(t)
	o, e := a.Retain(ctx, "one")
	if e != nil {
		t.Fatal(e)
	}
	sibling, e := a.Retain(ctx, "sibling")
	if e != nil {
		t.Fatal(e)
	}
	attachment, e := a.AttachRetained(o)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.HandoverRetained(ctx, o, attachment, nil); e == nil {
		t.Fatal("unaccepted transfer")
	}
	accepted, e := b.AcceptIncoming(o, "transfer-1", "build-A", conversation.CurrentCoexistenceDeclaration())
	if e != nil {
		t.Fatal(e)
	}
	if e = b.RetireCoexistence(ctx); !errors.Is(e, broker.ErrAdmissionBusy) {
		t.Fatalf("incoming retirement: %v", e)
	}
	if _, e = b.ClaimIncoming(ctx, accepted); e == nil {
		t.Fatal("claimed while source owns lock")
	}
	wrong := o
	wrong.Epoch = "wrong"
	bad, e := b.AcceptIncoming(wrong, "wrong-transfer", "build-A", conversation.CurrentCoexistenceDeclaration())
	if e != nil {
		t.Fatal(e)
	}
	if e = a.HandoverRetained(ctx, o, attachment, bad); e == nil {
		t.Fatal("wrong epoch accepted")
	}
	if e = a.HandoverRetained(ctx, o, attachment, accepted); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Retain(ctx, "one"); e == nil {
		t.Fatal("C stole reservation")
	}
	if _, e = a.AttachRetained(o); e == nil {
		t.Fatal("stale reconnect acquired")
	}
	if _, e = c.ClaimIncoming(ctx, accepted); !errors.Is(e, broker.ErrAdmissionStale) {
		t.Fatalf("C accepted B receipt: %v", e)
	}
	next, e := b.ClaimIncoming(ctx, accepted)
	if e != nil {
		t.Fatal(e)
	}
	if next.Epoch == o.Epoch {
		t.Fatal("epoch reused")
	}
	_, e = b.ClaimIncoming(ctx, accepted)
	var receipt *conversation.HandoverAlreadyClaimed
	if !errors.As(e, &receipt) || receipt.Epoch != next.Epoch {
		t.Fatalf("claim retry did not return durable receipt: %v", e)
	}
	if _, e = a.AdmitRetainedWork(sibling, broker.ForegroundWork); e != nil {
		t.Fatal("sibling disrupted", e)
	}
	if e = b.ReleaseRetained(ctx, next, nil); e != nil {
		t.Fatal(e)
	}
	fresh, e := c.Retain(ctx, "one")
	if e != nil {
		t.Fatal(e)
	}
	if fresh.Epoch == next.Epoch {
		t.Fatal("resume epoch reused")
	}
}
func TestCoexistenceLifecycleReadiness(t *testing.T) {
	ctx := context.Background()
	a, _, _ := lifecycleFixture(t)
	o, e := a.Retain(ctx, "one")
	if e != nil {
		t.Fatal(e)
	}
	x, _ := a.AttachRetained(o)
	y, _ := a.AttachRetained(o)
	if e = a.ReleaseRetained(ctx, o, x); !errors.Is(e, broker.ErrAttachmentEligibility) {
		t.Fatal(e)
	}
	y.Detach()
	for _, kind := range []broker.WorkKind{broker.ForegroundWork, broker.TaskAcceptanceWork, broker.TaskExecutionWork, broker.CallbackWork, broker.PersistenceWork} {
		ticket, e := a.AdmitRetainedWork(o, kind)
		if e != nil {
			t.Fatal(e)
		}
		if e = a.ReleaseRetained(ctx, o, x); !errors.Is(e, broker.ErrAdmissionBusy) {
			t.Fatal(e)
		}
		if kind == broker.PersistenceWork {
			_ = ticket.PersistenceFailed()
			if e = ticket.Complete(); !errors.Is(e, broker.ErrAdmissionBusy) {
				t.Fatal(e)
			}
			if e = a.ReleaseRetained(ctx, o, x); !errors.Is(e, broker.ErrAdmissionBusy) {
				t.Fatal(e)
			}
			_ = ticket.ResolvePersistence()
		}
		_ = ticket.Complete()
	}
	x.Detach()
	list, e := a.ListRetained()
	if e != nil || len(list) != 1 {
		t.Fatal(list, e)
	}
	closed := 0
	a.coexistence.options.CloseParticipation = func(context.Context) error { closed++; return nil }
	if e = a.RetireCoexistence(ctx); !errors.Is(e, broker.ErrAdmissionBusy) || closed != 0 {
		t.Fatal(e, closed)
	}
	a.coexistence.options.Cleanup = func(context.Context, broker.Owner) error {
		if e := a.RetireCoexistence(ctx); !errors.Is(e, broker.ErrAdmissionBusy) {
			t.Fatalf("cleanup ticket lost: %v", e)
		}
		return nil
	}
	if e = a.ReleaseRetained(ctx, o, nil); e != nil {
		t.Fatal(e)
	}
	if e = a.RetireCoexistence(ctx); e != nil {
		t.Fatal(e)
	}
	if closed != 1 {
		t.Fatal(closed)
	}
	if _, e = a.Retain(ctx, "new"); !errors.Is(e, broker.ErrAdmissionClosed) {
		t.Fatal(e)
	}
	if _, e = a.AttachRetained(o); e == nil {
		t.Fatal("retired reconnect")
	}
}
func TestCoexistenceLifecyclePersistenceFailureClosed(t *testing.T) {
	ctx := context.Background()
	a, _, _ := lifecycleFixture(t)
	o, e := a.Retain(ctx, "one")
	if e != nil {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e = a.ReleaseRetained(canceled, o, nil); e == nil {
		t.Fatal("expected persistence failure")
	}
	if _, e = a.AttachRetained(o); !errors.Is(e, broker.ErrAdmissionClosed) {
		t.Fatal(e)
	}
	if e = a.RetireCoexistence(ctx); !errors.Is(e, broker.ErrAdmissionBusy) {
		t.Fatal(e)
	}
	state, e := conversation.ReadOwnership(ctx, a.coexistence.store, "one")
	if e != nil || state.State != "owned" || state.Epoch != o.Epoch {
		t.Fatal(state, e)
	}
}
func TestCoexistenceAcquireTicketAndRuntimeIsolation(t *testing.T) {
	ctx := context.Background()
	a, b, _ := lifecycleFixture(t)
	a.coexistence.options.Reload = func(context.Context, conversation.Store, *conversation.Ownership) error {
		if e := a.RetireCoexistence(ctx); !errors.Is(e, broker.ErrAdmissionBusy) {
			t.Fatalf("acquisition ticket lost: %v", e)
		}
		return nil
	}
	o, e := a.Retain(ctx, "one")
	if e != nil {
		t.Fatal(e)
	}
	other, e := b.Retain(ctx, "other")
	if e != nil {
		t.Fatal(e)
	}
	closes := 0
	a.coexistence.options.CloseParticipation = func(context.Context) error { closes++; return nil }
	if e = a.ReleaseRetained(ctx, o, nil); e != nil {
		t.Fatal(e)
	}
	if e = a.RetireCoexistence(ctx); e != nil {
		t.Fatal(e)
	}
	ticket, e := b.AdmitRetainedWork(other, broker.ForegroundWork)
	if e != nil {
		t.Fatal("sibling participation disrupted", e)
	}
	_ = ticket.Complete()
	if closes != 1 {
		t.Fatal(closes)
	}
}
