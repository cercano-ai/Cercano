package server

import (
	"context"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestTakeoverPendingWritebackTimeoutRetainsSourceAndTicket(t *testing.T) {
	f, r := takeoverFixture(t)
	l := f.a.coexistence
	o := controlOwner(f.op)
	work, err := l.admission.AdmitWork(o, broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	confirm := takeoverOffer(t, f, r)
	ctx, cancel := context.WithTimeout(f.ctx, 100*time.Millisecond)
	defer cancel()
	if _, err = f.clients[f.c].LifecycleTakeover(ctx, confirm); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("pending writeback accepted: %v", err)
	}
	// Wait for server-side cancellation to finish accounting; the client deadline
	// alone does not prove the source handler has returned.
	deadline := time.Now().Add(time.Second)
	for {
		l.mu.Lock()
		reservation := l.closures[o]
		inactive := reservation != nil && reservation.control != nil && !reservation.control.active
		l.mu.Unlock()
		if inactive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("source did not finish handling the cancelled request")
		}
		time.Sleep(time.Millisecond)
	}
	state, err := conversation.ReadOwnership(f.ctx, l.store, o.Conversation)
	if err != nil || state.Owner != o.Host || state.Epoch != o.Epoch || state.State != "owned" {
		t.Fatalf("pending writeback changed ownership: %+v %v", state, err)
	}
	if retired, retained, open := l.admission.CheckRetained(o); retired || !retained || open {
		t.Fatalf("source must remain retained and fenced: %v %v %v", retired, retained, open)
	}
	l.mu.Lock()
	closure := l.closures[o].closure
	l.mu.Unlock()
	pending, uncertain, err := closure.Pending()
	if err != nil || pending != 1 || uncertain != 0 {
		t.Fatalf("pending producer ticket lost: %d %d %v", pending, uncertain, err)
	}
	// Only the producer's real completion releases the writeback ticket.
	if err = work.Complete(); err != nil {
		t.Fatal(err)
	}
	if _, err = f.clients[f.c].LifecycleTakeover(f.ctx, confirm); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("timed-out confirmation was reusable: %v", err)
	}
}

func TestTakeoverFreshConsentResumesExactPending(t *testing.T) {
	f, r := takeoverFixture(t)
	work, err := f.a.coexistence.admission.AdmitWork(controlOwner(f.op), broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	first := takeoverOffer(t, f, r)
	ctx, cancel := context.WithTimeout(f.ctx, 100*time.Millisecond)
	defer cancel()
	if _, err = f.clients[f.c].LifecycleTakeover(ctx, first); status.Code(err) != codes.DeadlineExceeded {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		l := f.a.coexistence
		l.mu.Lock()
		old := l.closures[controlOwner(f.op)]
		inactive := old != nil && old.control != nil && !old.control.active
		l.mu.Unlock()
		if inactive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancelled source handler still active")
		}
		time.Sleep(time.Millisecond)
	}
	if err = work.Complete(); err != nil {
		t.Fatal(err)
	}
	l := f.a.coexistence
	l.mu.Lock()
	exact := proto.Clone(l.closures[controlOwner(f.op)].control.request).(*pb.LifecycleControlRequest)
	l.mu.Unlock()
	changed := proto.Clone(exact).(*pb.LifecycleControlRequest)
	changed.HandoverId = "arbitrary-replacement"
	if _, err = f.clients[f.a].LifecycleTakeoverProbe(f.ctx, changed); err == nil {
		t.Fatal("replacement accepted")
	}
	fresh := takeoverOffer(t, f, r)
	got, err := f.clients[f.c].LifecycleTakeover(f.ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ready || got.OperationId != exact.HandoverId {
		t.Fatal(got)
	}
	if _, err = f.clients[f.c].LifecycleTakeover(f.ctx, first); status.Code(err) != codes.PermissionDenied {
		t.Fatal("obsolete consent accepted", err)
	}
}
