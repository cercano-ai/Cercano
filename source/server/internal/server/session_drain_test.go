package server

import (
	"cercano/source/server/pkg/agentclient"
	pb "cercano/source/server/pkg/proto"
	"context"
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
