package server

import (
	"context"
	"errors"
	"testing"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
)

func TestCoexistenceLifecycleUnresolvedCleanup(t *testing.T) {
	ctx := context.Background()
	a, _, _ := lifecycleFixture(t)
	o, e := a.Retain(ctx, "one")
	if e != nil {
		t.Fatal(e)
	}
	failure := errors.New("callback persistence unresolved")
	a.coexistence.options.Cleanup = func(context.Context, broker.Owner) error { return failure }
	if e = a.ReleaseRetained(ctx, o, nil); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	list, e := a.ListRetained()
	if e != nil || len(list) != 0 {
		t.Fatal(list, e)
	}
	if e = a.RetireCoexistence(ctx); !errors.Is(e, broker.ErrAdmissionBusy) {
		t.Fatalf("callback ticket vanished: %v", e)
	}
}
func TestCoexistenceLifecycleReloadFailure(t *testing.T) {
	ctx := context.Background()
	a, _, _ := lifecycleFixture(t)
	a.coexistence.options.Reload = func(context.Context, conversation.Store, *conversation.Ownership) error {
		return errors.New("pending outcomes unresolved")
	}
	if _, e := a.Retain(ctx, "one"); e == nil {
		t.Fatal("reload failure hidden")
	}
	list, e := a.ListRetained()
	if e != nil || len(list) != 1 {
		t.Fatal(list, e)
	}
	if _, e = a.AttachRetained(list[0]); e == nil {
		t.Fatal("unresolved acquisition admitted attachment")
	}
	if e = a.RetireCoexistence(ctx); !errors.Is(e, broker.ErrAdmissionBusy) {
		t.Fatal(e)
	}
}
func TestCoexistenceLifecycleRetireHookFailure(t *testing.T) {
	ctx := context.Background()
	a, _, _ := lifecycleFixture(t)
	failure := errors.New("participation close failed")
	a.coexistence.options.CloseParticipation = func(context.Context) error { return failure }
	if e := a.RetireCoexistence(ctx); !errors.Is(e, failure) {
		t.Fatal(e)
	}
	if _, e := a.Retain(ctx, "one"); !errors.Is(e, broker.ErrAdmissionClosed) {
		t.Fatal(e)
	}
}
