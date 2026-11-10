package server

import (
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"context"
	"errors"
	"testing"
)

// The store-level test injects both durable outcomes at Commit. This tests the
// coordinator's existing capability-plus-error boundary independently of SQL.
func TestRecoveryUncertainOutcomeNeverReady(t *testing.T) {
	ctx := context.Background()
	a, b, _ := lifecycleFixture(t)
	old, err := a.Retain(ctx, "uncertain")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.coexistence.owners["uncertain"].Close(); err != nil {
		t.Fatal(err)
	}
	l := b.coexistence
	before, err := conversation.ReadOwnership(ctx, l.store, "uncertain")
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("uncertain acknowledgement")
	_, err = l.acquireUsing(ctx, "uncertain", nil, nil, func() (*conversation.Ownership, error) {
		cap, e := conversation.RecoverDeadOwner(ctx, l.store, "uncertain", before, l.options.Host, func(context.Context) error { return nil })
		if e != nil {
			t.Fatal(e)
		}
		return cap, injected
	})
	if !errors.Is(err, injected) {
		t.Fatal(err)
	}
	cap := l.owners["uncertain"]
	if cap == nil || l.ready[ownerOf(cap)] {
		t.Fatal("lost lock or published Ready")
	}
	l.options.ControlAuthorize = func(context.Context) error { return nil }
	l.options.RecoveryGuard = func(context.Context, string) (func(), error) { return func() {}, nil }
	request := &pb.LifecycleRecoverRequest{Target: l.controlIdentity(), Conversation: "uncertain", Confirm: true, ExpectedOwner: &pb.LifecycleOwner{Conversation: old.Conversation, Host: old.Host, Epoch: old.Epoch}}
	for i := 0; i < 2; i++ {
		response, e := b.LifecycleRecover(ctx, request)
		if e == nil || response.GetReady() {
			t.Fatalf("uncertain retry ready: %v %v", response, e)
		}
		state, e := conversation.ReadOwnership(ctx, l.store, "uncertain")
		if e != nil || state.Epoch != cap.Epoch() {
			t.Fatalf("duplicate acquisition: %+v %v", state, e)
		}
	}
}
