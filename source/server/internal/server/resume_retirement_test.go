package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/broker"
	pb "cercano/source/server/pkg/proto"
)

func releaseResumeAndRetire(t *testing.T, b *Server, got *pb.LifecycleControlResponse) {
	t.Helper()
	if !got.GetReady() {
		t.Fatalf("not ready: %v", got)
	}
	if err := b.ReleaseRetained(context.Background(), broker.Owner{Conversation: got.Owner.Conversation, Host: got.Owner.Host, Epoch: got.Owner.Epoch}, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.RetireCoexistence(context.Background()); err != nil {
		t.Fatalf("retirement after release: %v (acceptances=%d)", err, len(b.coexistence.incoming))
	}
}

func TestDestinationResumeUnpublishedAcceptanceRetirement(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	serveCutoverFixture(t, a)
	bc, endpoint := serveCutoverFixture(t, b)
	ctx := context.Background()
	if _, err := a.Retain(ctx, "chat"); err != nil {
		t.Fatal(err)
	}
	source := &resumeSource{Server: a}
	setResumeSource(b, a, serveResumeSource(t, source))
	dir := filepath.Dir(resumeJournal(b.coexistence, "chat"))
	if err := os.MkdirAll(dir, 0500); err != nil {
		t.Fatal(err)
	}
	if _, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint)); err == nil {
		t.Fatal("expected definitive publication failure")
	}
	if source.calls != 0 {
		t.Fatal("unpublished operation contacted source")
	}
	t.Logf("acceptances after definitive failure: %d", len(b.coexistence.incoming))
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	got, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
	if err != nil {
		t.Fatal(err)
	}
	releaseResumeAndRetire(t, b, got)
}

func TestDestinationResumeRecoverySupersedesParticipation(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-process", true: "restart"}[restart], func(t *testing.T) {
			a, b, _ := lifecycleFixture(t)
			serveCutoverFixture(t, a)
			bc, endpoint := serveCutoverFixture(t, b)
			ctx := context.Background()
			old, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			dead := false
			b.coexistence.options.RecoveryGuard = func(context.Context, string) (func(), error) {
				if !dead {
					return nil, errors.New("still alive")
				}
				return func() {}, nil
			}
			source := &resumeSource{Server: a, legacy: true, blocked: true}
			setResumeSource(b, a, serveResumeSource(t, source))
			offer, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
			if err != nil || !offer.GetConfirmationRequired() {
				t.Fatalf("offer: %v %v", offer, err)
			}
			if len(b.coexistence.incoming) != 1 {
				t.Fatal("missing participation")
			}
			// Closing the old capability simulates source death without using a process.
			if err := a.coexistence.owners["chat"].Close(); err != nil {
				t.Fatal(err)
			}
			dead = true
			if restart {
				b, bc, endpoint = restartResumeFixture(t, b)
			}
			got, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
			if err != nil {
				t.Fatal(err)
			}
			if got.Owner.Epoch == old.Epoch {
				t.Fatal("recovery reused source epoch")
			}
			releaseResumeAndRetire(t, b, got)
		})
	}
}

func TestDestinationResumeUncertainPublicationRetainsIdentity(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-process", true: "restart"}[restart], func(t *testing.T) {
			a, b, _ := lifecycleFixture(t)
			serveCutoverFixture(t, a)
			bc, endpoint := serveCutoverFixture(t, b)
			ctx := context.Background()
			if _, err := a.Retain(ctx, "chat"); err != nil {
				t.Fatal(err)
			}
			source := &resumeSource{Server: a}
			setResumeSource(b, a, serveResumeSource(t, source))
			b.coexistence.resumeJournalSync = func(*os.File) error { return errors.New("injected post-rename directory sync failure") }
			for i := 0; i < 2; i++ {
				if _, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint)); err == nil {
					t.Fatal("expected uncertain publication")
				}
			}
			if source.calls != 0 {
				t.Fatal("uncertain journal contacted source")
			}
			data, err := os.ReadFile(resumeJournal(b.coexistence, "chat"))
			if err != nil {
				t.Fatal(err)
			}
			var op pb.LifecycleControlRequest
			if err := json.Unmarshal(data, &op); err != nil {
				t.Fatal(err)
			}
			if op.HandoverId == "" || op.Receipt == "" || len(b.coexistence.incoming) != 1 {
				t.Fatal("lost recoverable operation")
			}
			for _, acceptance := range b.coexistence.incoming {
				if acceptance.h.HandoverID != op.HandoverId || acceptance.receipt != op.Receipt {
					t.Fatal("changed acceptance")
				}
			}
			if err := b.RetireCoexistence(ctx); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("uncertain operation retired: %v", err)
			}
			b.coexistence.resumeJournalSync = nil
			if restart {
				b, bc, endpoint = restartResumeFixture(t, b)
			}
			got, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
			if err != nil {
				t.Fatal(err)
			}
			if got.OperationId != op.HandoverId {
				t.Fatal("retry replaced uncertain identity")
			}
			releaseResumeAndRetire(t, b, got)
		})
	}
}

func TestDestinationResumeRecoveryPreservesOtherParticipation(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	serveCutoverFixture(t, a)
	bc, endpoint := serveCutoverFixture(t, b)
	ctx := context.Background()
	old, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	source := &resumeSource{Server: a, legacy: true, blocked: true}
	setResumeSource(b, a, serveResumeSource(t, source))
	if offer, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint)); err != nil || !offer.GetConfirmationRequired() {
		t.Fatalf("offer: %v %v", offer, err)
	}
	other, err := b.AcceptIncoming(old, "other-operation", a.coexistence.options.Build, a.coexistence.options.Contract)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.coexistence.owners["chat"].Close(); err != nil {
		t.Fatal(err)
	}
	b.coexistence.options.RecoveryGuard = func(context.Context, string) (func(), error) { return func() {}, nil }
	got, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
	if err != nil {
		t.Fatal(err)
	}
	if b.coexistence.incoming[other.h] != other || len(b.coexistence.incoming) != 1 {
		t.Fatal("recovery erased unrelated participation")
	}
	if err := b.ReleaseRetained(ctx, broker.Owner{Conversation: got.Owner.Conversation, Host: got.Owner.Host, Epoch: got.Owner.Epoch}, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.RetireCoexistence(ctx); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("other operation no longer fences retirement: %v", err)
	}
}
