package compactiongen

import (
	"context"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compactor"
)

func TestOwnedCancellationWaitsForCallback(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner := broker.Owner{Conversation: "c", Host: "host", Epoch: "epoch"}
	a := broker.NewAdmission("host")
	if err := a.RegisterOwner(owner); err != nil {
		t.Fatal(err)
	}
	entered, cancelled, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	g := New(nil, nil, compactor.Config{}, nil, 0)
	g.SetEnabled(true)
	g.SetToolElisionOnly(true)
	g.SetElideOnlyFn(func(ctx context.Context, _ string) (int, int, int, bool, error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-finish
		return 0, 0, 0, false, ctx.Err()
	})
	work, err := g.ScheduleOwned(parent, owner, func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
		ticket, err := a.AdmitWork(o, k)
		return ctx, ticket, err
	})
	if err != nil {
		t.Fatal(err)
	}
	wait := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("barrier timed out")
		}
	}
	wait(entered)
	cancel()
	wait(cancelled)
	expired, stop := context.WithCancel(context.Background())
	stop()
	if err := g.Close(expired); !errors.Is(err, context.Canceled) {
		t.Fatalf("drain abandoned callback: %v", err)
	}
	if _, err := a.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("cancel released callback: %v", err)
	}
	close(finish)
	wait(work.Done)
	if err := g.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The callback could have partially persisted before cancellation. Drain is
	// not reconciliation and must not erase this conservative readiness blocker.
	if _, err := a.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("drain erased unresolved outcome: %v", err)
	}
}
