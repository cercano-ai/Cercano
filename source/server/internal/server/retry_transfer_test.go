package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestRetryTransferMissingSource(t *testing.T) {
	f := newExactRecoveryFixture(t)
	before := f.snapshot(t)
	_, err := f.clients[f.a].LifecycleRetryRelinquish(f.ctx, f.op)
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected existing-state rejection: %v", err)
	}
	if before != f.snapshot(t) {
		t.Fatal("rejection mutated state")
	}
}

func TestRetryTransferTimeout(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "uncertain"}[uncertain], func(t *testing.T) {
			f := newExactRecoveryFixture(t)
			held, err := f.a.AdmitRetainedWork(controlOwner(f.op), broker.PersistenceWork)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(f.ctx, 80*time.Millisecond)
			_, err = f.clients[f.a].LifecycleForceRelinquish(ctx, f.op)
			cancel()
			if status.Code(err) != codes.DeadlineExceeded {
				t.Fatal(err)
			}
			if uncertain {
				if err = held.PersistenceFailed(); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel = context.WithTimeout(f.ctx, 80*time.Millisecond)
			_, err = f.clients[f.a].LifecycleRetryRelinquish(ctx, f.op)
			cancel()
			expected := codes.DeadlineExceeded
			if uncertain {
				expected = codes.FailedPrecondition
			}
			if status.Code(err) != expected {
				t.Fatalf("blocking: %v", err)
			}
			if uncertain {
				return
			}
			if err = held.Complete(); err != nil {
				t.Fatal(err)
			}
			if _, err = f.clients[f.a].LifecycleRetryRelinquish(f.ctx, f.op); err != nil {
				t.Fatal(err)
			}
			got, err := f.clients[f.b].LifecycleRetryClaim(f.ctx, f.claim())
			if err != nil || !got.GetCurrentOwner() {
				t.Fatalf("claim: %v %v", got, err)
			}
			before := f.snapshot(t)
			if _, err = f.clients[f.a].LifecycleRetryRelinquish(f.ctx, f.op); err != nil {
				t.Fatal(err)
			}
			again, err := f.clients[f.b].LifecycleRetryClaim(f.ctx, f.claim())
			if err != nil || !proto.Equal(got, again) {
				t.Fatalf("retry: %v %v", again, err)
			}
			if before != f.snapshot(t) {
				t.Fatal("completed retry mutated state")
			}
			if err = f.b.ReleaseRetained(f.ctx, controlOwner(&pb.LifecycleControlRequest{Owner: got.Owner}), nil); err != nil {
				t.Fatal(err)
			}
			before = f.snapshot(t)
			historical, err := f.clients[f.b].LifecycleRetryClaim(f.ctx, f.claim())
			if err != nil || historical.GetCurrentOwner() || !proto.Equal(got.Owner, historical.GetOwner()) {
				t.Fatalf("historical: %v %v", historical, err)
			}
			if before != f.snapshot(t) {
				t.Fatal("historical retry reacquired")
			}
		})
	}
}

func TestRetryTransferReject(t *testing.T) {
	for _, which := range []string{"receipt", "wrong-receipt", "source", "source-host", "source-store", "source-contract", "source-schema", "epoch", "conversation", "owner-host", "destination", "destination-build", "operation", "target", "management", "acceptance", "missing-reservation"} {
		t.Run(which, func(t *testing.T) {
			f := newExactRecoveryFixture(t)
			if which != "missing-reservation" {
				f.reserve(t)
			}
			r := proto.Clone(f.op).(*pb.LifecycleControlRequest)
			switch which {
			case "receipt":
				r.Receipt = ""
			case "wrong-receipt":
				r.Receipt = "wrong"
			case "source-host":
				r.Source.Host = "wrong"
			case "source-store":
				r.Source.Store = "wrong"
			case "source-contract":
				r.Source.Contract = "wrong"
			case "source-schema":
				r.Source.Schema++
			case "conversation":
				r.Owner.Conversation = "wrong"
			case "owner-host":
				r.Owner.Host = "wrong"
			case "destination-build":
				r.Destination.Build = "wrong"
			case "management":
				r.Management = false
			case "source":
				r.Source.Build = "wrong"
			case "epoch":
				r.Owner.Epoch = "wrong"
			case "destination":
				r.Destination = f.c.coexistence.controlIdentity()
			case "operation":
				r.HandoverId = "wrong"
			case "target":
				r.Target = f.c.coexistence.controlIdentity()
			case "acceptance":
				f.b.coexistence.mu.Lock()
				clear(f.b.coexistence.incoming)
				f.b.coexistence.mu.Unlock()
			}
			before := f.snapshot(t)
			if which != "missing-reservation" {
				if _, err := f.clients[f.a].LifecycleRetryRelinquish(f.ctx, r); err == nil {
					t.Fatal("source accepted mismatch")
				}
			}
			if which != "target" {
				r.Target = f.b.coexistence.controlIdentity()
			}
			if _, err := f.clients[f.b].LifecycleRetryClaim(f.ctx, r); err == nil {
				t.Fatal("destination accepted mismatch")
			}
			if before != f.snapshot(t) {
				t.Fatal("rejection mutated state")
			}
		})
	}
}

func TestRetryTransferUnresolvedDestination(t *testing.T) {
	f := newExactRecoveryFixture(t)
	f.reserve(t)
	f.b.coexistence.options.Reload = func(context.Context, conversation.Store, *conversation.Ownership) error {
		return errors.New("fixture reload unresolved")
	}
	if _, err := f.clients[f.b].LifecycleRetryClaim(f.ctx, f.claim()); err == nil {
		t.Fatal("fixture failure not exercised")
	}
	before := f.snapshot(t)
	if got, err := f.clients[f.b].LifecycleRetryClaim(f.ctx, f.claim()); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unresolved destination reported complete: %v %v", got, err)
	}
	if before != f.snapshot(t) {
		t.Fatal("unresolved retry mutated state")
	}
}

// A preflight is not authority for either following local transition.
func TestRetryTransferPreflightChanges(t *testing.T) {
	for _, change := range []string{"acceptance", "source-owner"} {
		t.Run(change, func(t *testing.T) {
			f := newExactRecoveryFixture(t)
			if change == "acceptance" {
				f.reserve(t)
			}
			if _, err := f.clients[f.b].LifecycleRetryReceipt(f.ctx, f.claim()); err != nil {
				t.Fatal(err)
			}
			if change == "acceptance" {
				f.b.coexistence.mu.Lock()
				clear(f.b.coexistence.incoming)
				f.b.coexistence.mu.Unlock()
			} else {
				if err := f.a.ReleaseRetained(f.ctx, controlOwner(f.op), nil); err != nil {
					t.Fatal(err)
				}
				if _, err := f.c.Retain(f.ctx, "chat"); err != nil {
					t.Fatal(err)
				}
			}
			before := f.snapshot(t)
			if _, err := f.clients[f.a].LifecycleRetryRelinquish(f.ctx, f.op); err == nil {
				t.Fatal("stale preflight authorized source")
			}
			if _, err := f.clients[f.b].LifecycleRetryClaim(f.ctx, f.claim()); err == nil {
				t.Fatal("stale preflight authorized claim")
			}
			if before != f.snapshot(t) {
				t.Fatal("rejection mutated state")
			}
		})
	}
}

func TestRetryTransferSourceCommitUnresolved(t *testing.T) {
	f := newExactRecoveryFixture(t)
	held, err := f.a.AdmitRetainedWork(controlOwner(f.op), broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 80*time.Millisecond)
	_, err = f.clients[f.a].LifecycleForceRelinquish(ctx, f.op)
	cancel()
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatal(err)
	}
	if err = held.Complete(); err != nil {
		t.Fatal(err)
	}
	f.a.coexistence.options.Cleanup = func(context.Context, broker.Owner) error { return errors.New("fixture cleanup unresolved") }
	if _, err = f.clients[f.a].LifecycleRetryRelinquish(f.ctx, f.op); err == nil {
		t.Fatal("cleanup failure not exercised")
	}
	before := f.snapshot(t)
	if _, err = f.clients[f.a].LifecycleRetryRelinquish(f.ctx, f.op); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unresolved source reported complete: %v", err)
	}
	if before != f.snapshot(t) {
		t.Fatal("retry repeated uncertain write")
	}
}

func TestRetryTransferClientIntegration(t *testing.T) {
	f := newExactRecoveryFixture(t)
	held, err := f.a.AdmitRetainedWork(controlOwner(f.op), broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 80*time.Millisecond)
	_, err = f.clients[f.a].LifecycleForceRelinquish(ctx, f.op)
	cancel()
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatal(err)
	}
	if err = held.Complete(); err != nil {
		t.Fatal(err)
	}
	src := &agentclient.LifecycleControls{AgentClient: f.clients[f.a]}
	dst := &agentclient.LifecycleControls{AgentClient: f.clients[f.b]}
	original := proto.Clone(f.op)
	got, err := src.RetryTransfer(f.ctx, dst, f.op)
	if err != nil || !got.GetCurrentOwner() {
		t.Fatalf("retry: %v %v", got, err)
	}
	before := f.snapshot(t)
	again, err := src.RetryTransfer(f.ctx, dst, f.op)
	if err != nil || !proto.Equal(got, again) || !proto.Equal(original, f.op) || before != f.snapshot(t) {
		t.Fatalf("completed retry changed operation: %v %v", again, err)
	}
}
