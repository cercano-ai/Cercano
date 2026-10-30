package server

import (
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
	pb "cercano/source/server/pkg/proto"
	"context"
	"database/sql"
	"errors"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"net"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
)

func TestSessionCloseBusyDrain(t *testing.T) {
	// Host selection lives outside this lifecycle. Closing does not consult it:
	// the same owner-accounting path runs on current and superseded hosts.
	{
		s, next, _ := lifecycleFixture(t)
		o, err := s.Retain(t.Context(), "same-conversation")
		if err != nil {
			t.Fatal(err)
		}
		terminal := "pane"
		attachment, err := s.coexistence.admission.AttachTerminal(o, terminal)
		if err != nil {
			t.Fatal(err)
		}
		workctx, task, err := s.BeginRetainedWork(t.Context(), o, broker.TaskAcceptanceWork)
		if err != nil {
			t.Fatal(err)
		}
		released := make(chan struct{})
		l := s.coexistence
		l.options.Cleanup = func(context.Context, broker.Owner) error { close(released); return nil }
		l.options.ControlAuthorize = func(context.Context) error { return nil } // disposable fixture only
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(agentclient.PinnedHostMetadata, o.Host, agentclient.PinnedConversationMetadata, o.Conversation, agentclient.PinnedEpochMetadata, o.Epoch, agentclient.PinnedTerminalMetadata, terminal))
		ctx = peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}})
		response, err := s.LifecycleRelease(ctx, &pb.LifecycleControlRequest{Target: l.controlIdentity(), Source: l.controlIdentity(), Owner: wireOwner(o)})
		if err == nil && (response == nil || response.Pending) {
			t.Fatal("normal session close not acknowledged")
		}
		if err != nil {
			t.Fatalf("explicit busy session close refused: %v", err)
		}
		attachment.Detach() // transport cleanup after explicit close is harmless
		if _, err = l.admission.AttachTerminal(o, terminal); !errors.Is(err, broker.ErrAdmissionClosed) {
			t.Fatalf("new attachment: %v", err)
		}
		if _, _, err = s.BeginRetainedWork(workctx, o, broker.ForegroundWork); !errors.Is(err, broker.ErrAdmissionClosed) {
			t.Fatalf("new foreground: %v", err)
		}
		if _, err = s.resolvePreparedTaskOwner(workctx, o.Conversation); err != nil {
			t.Fatalf("descendant authority: %v", err)
		}
		_, child, err := s.BeginRetainedWork(workctx, o, broker.TaskAcceptanceWork)
		if err != nil {
			t.Fatalf("independent descendant: %v", err)
		}
		if err = task.Complete(); err != nil {
			t.Fatal(err)
		}
		if err = child.Advance(broker.PersistenceWork); err != nil {
			t.Fatal(err)
		}
		if err = child.PersistenceFailed(); err != nil {
			t.Fatal(err)
		}
		if err = child.Complete(); !errors.Is(err, broker.ErrAdmissionBusy) {
			t.Fatalf("failure blocker lost: %v", err)
		}
		if _, err = next.Retain(t.Context(), o.Conversation); err == nil {
			t.Fatal("ownership transferred early")
		}
		if err = s.RetireCoexistence(t.Context()); !errors.Is(err, broker.ErrAdmissionBusy) {
			t.Fatalf("retirement before drain: %v", err)
		}
		select {
		case <-released:
			t.Fatal("released despite failed persistence")
		default:
		}
		// Simulate explicit reconciliation; completion, not a timer, triggers release.
		if err = child.ResolvePersistence(); err != nil {
			t.Fatal(err)
		}
		if err = child.Complete(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-released:
		case <-time.After(5 * time.Second):
			t.Fatal("completion did not release owner")
		}
		retained, err := s.ListRetained() // waits for release cleanup under lifecycle lock
		if err != nil || len(retained) != 0 {
			t.Fatalf("retained after drain: %v %v", retained, err)
		}
		if _, err = next.Retain(t.Context(), o.Conversation); err != nil {
			t.Fatalf("durable owner not released: %v", err)
		}
		if err = s.RetireCoexistence(t.Context()); err != nil {
			t.Fatalf("retire drained host: %v", err)
		}
	}
}

func TestSessionDisconnectRetainsAfterCompletion(t *testing.T) {
	s, _, _ := lifecycleFixture(t)
	o, err := s.Retain(t.Context(), "disconnected")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.coexistence.admission.AttachTerminal(o, "pane")
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.AdmitRetainedWork(o, broker.TaskAcceptanceWork)
	if err != nil {
		t.Fatal(err)
	}
	a.Detach()
	if err = task.Complete(); err != nil {
		t.Fatal(err)
	}
	retained, err := s.ListRetained()
	if err != nil || len(retained) != 1 {
		t.Fatalf("disconnect released owner: %v %v", retained, err)
	}
	if err = s.RetireCoexistence(t.Context()); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("disconnected retirement: %v", err)
	}
}

// Fail the durable transition reached by the real WhenDrained callback, not
// Cleanup (which runs only after durable ownership has already been released).
func TestSessionCloseAsyncDurableReleaseFailure(t *testing.T) {
	s, next, _ := lifecycleFixture(t)
	l := s.coexistence
	o, err := s.Retain(t.Context(), "async-release-failure")
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := l.admission.AttachTerminal(o, "pane")
	if err != nil {
		t.Fatal(err)
	}
	defer attachment.Detach()
	workctx, work, err := s.BeginRetainedWork(context.Background(), o, broker.TaskAcceptanceWork)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", l.options.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TRIGGER fail_async_release BEFORE UPDATE ON conversation_ownership WHEN NEW.conversation_id='async-release-failure' AND NEW.state='released' BEGIN SELECT RAISE(FAIL, 'injected async release failure'); END`); err != nil {
		t.Fatal(err)
	}
	cleaned := make(chan struct{}, 1)
	l.options.Cleanup = func(context.Context, broker.Owner) error { cleaned <- struct{}{}; return nil }
	request, cancel := context.WithCancel(t.Context())
	defer cancel()
	terminal := "pane"
	l.mu.Lock()
	err = l.relinquishLocked(request, o, nil, &terminal, nil)
	reservation := l.closures[o]
	l.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if reservation == nil {
		t.Fatal("missing drainage reservation")
	}
	cancel() // Transport lifetime must not cancel independent accepted work.
	if workctx.Err() != nil {
		t.Fatalf("independent work canceled: %v", workctx.Err())
	}
	_, child, err := s.BeginRetainedWork(workctx, o, broker.CallbackWork)
	if err != nil {
		t.Fatal(err)
	}
	if err = work.Complete(); err != nil {
		t.Fatal(err)
	}
	if _, err = next.Retain(t.Context(), o.Conversation); err == nil {
		t.Fatal("ownership transferred before drainage")
	}
	if err = child.Complete(); err != nil {
		t.Fatal(err)
	}
	// Advance is non-destructive and reports Busy only once the asynchronous
	// failure has marked the host ticket unresolved. The lifecycle lock also
	// joins the callback's critical section before inspecting its retained state.
	deadline := time.Now().Add(5 * time.Second)
	for {
		l.mu.Lock()
		err = reservation.ticket.Advance(broker.PersistenceWork)
		l.mu.Unlock()
		if errors.Is(err, broker.ErrAdmissionBusy) {
			break
		}
		if err != nil {
			t.Fatalf("host ticket lost: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("async durable failure did not retain unresolved host blocker")
		}
		time.Sleep(time.Millisecond)
	}
	l.mu.Lock()
	retained := l.owners[o.Conversation]
	sameReservation := l.closures[o] == reservation
	l.mu.Unlock()
	if retained == nil || ownerOf(retained) != o || !sameReservation {
		t.Fatal("failed callback dropped exact ownership/closure")
	}
	state, err := conversation.ReadOwnership(t.Context(), l.store, o.Conversation)
	if err != nil || state.State != "owned" {
		t.Fatalf("durable owner released: %+v %v", state, err)
	}
	if _, err = next.Retain(t.Context(), o.Conversation); err == nil {
		t.Fatal("failed release unlocked owner")
	}
	if _, err = l.admission.AdmitWork(o, broker.ForegroundWork); !errors.Is(err, broker.ErrAdmissionClosed) {
		t.Fatalf("foreground reopened: %v", err)
	}
	if _, err = l.admission.AttachTerminal(o, "new-pane"); !errors.Is(err, broker.ErrAdmissionClosed) {
		t.Fatalf("attachment reopened: %v", err)
	}
	if err = reservation.ticket.Complete(); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("unresolved persistence completed: %v", err)
	}
	if err = s.RetireCoexistence(t.Context()); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("failed release allowed retirement: %v", err)
	}
	select {
	case <-cleaned:
		t.Fatal("cleanup ran after failed durable release")
	default:
	}
	if workctx.Err() != nil {
		t.Fatalf("callback canceled independent work: %v", workctx.Err())
	}
}
