package recap

import (
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"context"
	"errors"
	"testing"
	"time"
)

// Characterizes the legacy API before opt-in admission: timers are not broker
// work, and the callback starts from Background rather than caller authority.
func TestLegacySchedulingAdmissionAndContextGap(t *testing.T) {
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "capability")
	a := broker.NewAdmission("host")
	owner := broker.Owner{Conversation: "c", Host: "host", Epoch: "epoch"}
	if err := a.RegisterOwner(owner); err != nil {
		t.Fatal(err)
	}
	seen := make(chan any, 2)
	fs := &fakeStore{turns: []conversation.Turn{{Content: "hi"}}, updated: make(chan string, 2)}
	g := New(fs, func(ctx context.Context, _ string) (string, error) { seen <- ctx.Value(key{}); return "summary", nil }, 30*time.Millisecond, 10)
	defer g.Close(context.Background())
	_ = parent // Schedule cannot carry it.
	g.Schedule("c")
	closure, err := a.CloseOwner(owner, nil)
	if err != nil {
		t.Fatalf("legacy unexpectedly counted timer: %v", err)
	}
	defer closure.Reopen()
	select {
	case value := <-seen:
		if value != nil {
			t.Fatal(value)
		}
	case <-time.After(time.Second):
		t.Fatal("timer did not run")
	}
}

func TestOwnedRejectedTransitionRetainsBlocker(t *testing.T) {
	owner := broker.Owner{Conversation: "c", Host: "host", Epoch: "epoch"}
	a := broker.NewAdmission("host")
	if err := a.RegisterOwner(owner); err != nil {
		t.Fatal(err)
	}
	g := New(nil, nil, 0, 10)
	defer g.Close(context.Background())
	work, err := g.ScheduleOwned(context.Background(), owner, func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
		ticket, err := a.AdmitWork(o, k)
		if err != nil {
			return nil, nil, err
		}
		if err := ticket.Advance(broker.PersistenceWork); err != nil {
			t.Fatal(err)
		}
		if err := ticket.PersistenceFailed(); err != nil {
			t.Fatal(err)
		}
		return ctx, ticket, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-work.Done
	if !errors.Is(work.Err, broker.ErrAdmissionBusy) {
		t.Fatalf("rejected transition: %v", work.Err)
	}
	if _, err := a.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("lost blocker: %v", err)
	}
}

func TestOwnedAdmissionsDoNotCoalesceAuthorityOrCancellation(t *testing.T) {
	owner := broker.Owner{Conversation: "c", Host: "host", Epoch: "epoch"}
	a := broker.NewAdmission("host")
	if err := a.RegisterOwner(owner); err != nil {
		t.Fatal(err)
	}
	type key struct{}
	seen := make(chan any, 4)
	fs := &fakeStore{turns: []conversation.Turn{{Content: "hello"}}, updated: make(chan string, 2)}
	g := New(fs, func(ctx context.Context, _ string) (string, error) { seen <- ctx.Value(key{}); return "summary", nil }, 20*time.Millisecond, 10)
	defer g.Close(context.Background())
	begin := func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
		ticket, err := a.AdmitWork(o, k)
		return ctx, ticket, err
	}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "cancelled"))
	first, err := g.ScheduleOwned(parent, owner, begin)
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.ScheduleOwned(context.WithValue(context.Background(), key{}, "retained"), owner, begin)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	<-first.Done
	if !errors.Is(first.Err, context.Canceled) {
		t.Fatal(first.Err)
	}
	select {
	case <-second.Done:
	case <-time.After(time.Second):
		t.Fatal("not completed")
	}
	if second.Err != nil {
		t.Fatal(second.Err)
	}
	for range 2 {
		if value := <-seen; value != "retained" {
			t.Fatalf("authority replaced: %v", value)
		}
	}
	if _, err := a.CloseOwner(owner, nil); err != nil {
		t.Fatal(err)
	}
}
