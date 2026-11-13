package server

import (
	"context"
	"os"
	"testing"

	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/protobuf/proto"
)

func TestResumeRecoveryExactProof(t *testing.T) {
	for _, mismatch := range []string{"none", "source", "epoch", "destination", "conversation", "acceptance-receipt", "handover", "build", "no-recovery"} {
		t.Run(mismatch, func(t *testing.T) {
			ctx := context.Background()
			a, b, c := lifecycleFixture(t)
			old, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			acceptance, err := b.AcceptIncoming(old, "operation", a.coexistence.options.Build, b.coexistence.options.Contract)
			if err != nil {
				t.Fatal(err)
			}
			op := &pb.LifecycleControlRequest{Owner: wireOwner(old), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), HandoverId: "operation", Receipt: acceptance.receipt}
			unrelated, err := b.AcceptIncoming(old, "unrelated", a.coexistence.options.Build, b.coexistence.options.Contract)
			if err != nil {
				t.Fatal(err)
			}
			before, err := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
			if err != nil {
				t.Fatal(err)
			}
			if err = a.coexistence.owners["chat"].Close(); err != nil {
				t.Fatal(err)
			}
			if mismatch != "no-recovery" {
				recovered, e := conversation.RecoverDeadOwner(ctx, b.coexistence.store, "chat", before, "B", func(context.Context) error { return nil })
				if e != nil {
					t.Fatal(e)
				}
				if e = recovered.Release(ctx); e != nil {
					t.Fatal(e)
				}
				// Historical receipts must remain evidence even after a foreign acquisition.
				if _, e = c.Retain(ctx, "chat"); e != nil {
					t.Fatal(e)
				}
			}
			switch mismatch {
			case "source":
				op.Owner.Host = "other"
				op.Source = proto.Clone(op.Source).(*pb.LifecycleIdentity)
				op.Source.Host = "other"
			case "epoch":
				op.Owner.Epoch = "other"
			case "destination":
				op.Destination = c.coexistence.controlIdentity()
			case "conversation":
				op.Owner.Conversation = "other"
			case "acceptance-receipt":
				op.Receipt = "other"
			case "handover":
				op.HandoverId = "other"
			case "build":
				op.Source.Build = "other"
			}
			if err = saveResume(resumeJournal(b.coexistence, "chat"), op); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				release, gateErr := b.coexistence.enterResume(ctx, "chat")
				if gateErr != nil {
					t.Fatal(gateErr)
				}
				proven, e := b.coexistence.reconcileRecoveredResume(ctx, "chat")
				release()
				invalid := mismatch == "destination" || mismatch == "conversation"
				if (e != nil) != invalid {
					t.Fatalf("proof error: %v", e)
				}
				wantProof := mismatch == "none" || mismatch == "acceptance-receipt" || mismatch == "handover" || mismatch == "build"
				if proven != wantProof {
					t.Fatalf("proof=%v want=%v", proven, wantProof)
				}
				if (b.coexistence.incoming[acceptance.h] == nil) != (mismatch == "none") {
					t.Fatal("incorrect exact acceptance cleanup")
				}
				if b.coexistence.incoming[unrelated.h] != unrelated {
					t.Fatal("unrelated acceptance cleared")
				}
			}
		})
	}
}

// A committed recovery followed by a process crash permits journal cleanup,
// not acquisition of the recovered capability from historical evidence alone.
func TestResumeRecoveryRestartCleanupDoesNotGrantOwnership(t *testing.T) {
	ctx := reproLoopbackContext()
	a, b, _ := lifecycleFixture(t)
	old, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	acceptance, err := b.AcceptIncoming(old, "operation", a.coexistence.options.Build, b.coexistence.options.Contract)
	if err != nil {
		t.Fatal(err)
	}
	op := &pb.LifecycleControlRequest{Owner: wireOwner(old), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), HandoverId: "operation", Receipt: acceptance.receipt}
	if err = saveResume(resumeJournal(b.coexistence, "chat"), op); err != nil {
		t.Fatal(err)
	}
	before, err := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.coexistence.owners["chat"].Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := conversation.RecoverDeadOwner(ctx, b.coexistence.store, "chat", before, "B", func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = recovered.Close(); err != nil {
		t.Fatal(err)
	}
	b, _, endpoint := restartResumeFixture(t, b)
	synced := 0
	b.coexistence.resumeJournalSync = func(f *os.File) error { synced++; return f.Sync() }
	for i := 0; i < 2; i++ {
		got, e := b.LifecycleResume(ctx, resumeRequest(b, endpoint))
		if e == nil || got.GetReady() {
			t.Fatalf("historical receipt granted readiness: %v %v", got, e)
		}
	}
	if synced < 2 {
		t.Fatal("restart retries did not reconcile durable receipt")
	}
	after, err := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
	if err != nil || after.Owner != "B" || after.Epoch != recovered.Epoch() {
		t.Fatalf("cleanup changed owner: %+v %v", after, err)
	}
	if len(b.coexistence.owners) != 0 {
		t.Fatal("cleanup granted ownership")
	}
}
