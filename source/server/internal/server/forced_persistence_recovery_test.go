package server

import (
	"context"
	"strings"
	"testing"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const sixUnresolved = "forced cutover fenced old clients; 6 unresolved persistence outcomes remain (not evidence of running work)"

func TestForcedPersistenceRefusalClassification(t *testing.T) {
	for _, message := range []string{
		sixUnresolved,
		strings.Replace(sixUnresolved, "6 unresolved", "1 unresolved", 1),
		strings.Replace(sixUnresolved, "6 unresolved", "123 unresolved", 1),
	} {
		if !isForcedPersistenceRefusal(status.Error(codes.FailedPrecondition, message)) {
			t.Fatalf("refused canonical message %q", message)
		}
	}
	for _, message := range []string{
		"unresolved persistence outcomes remain", "wrong identity", "unresolved commit",
		"prefix " + sixUnresolved, sixUnresolved + " extra", sixUnresolved + "\n",
		strings.Replace(sixUnresolved, "6 unresolved", "0 unresolved", 1),
		strings.Replace(sixUnresolved, "6 unresolved", "06 unresolved", 1),
		strings.Replace(sixUnresolved, "6 unresolved", "-6 unresolved", 1),
		strings.Replace(sixUnresolved, "6 unresolved", "+6 unresolved", 1),
		strings.Replace(sixUnresolved, "6 unresolved", "6.0 unresolved", 1),
		strings.Replace(sixUnresolved, " (not evidence of running work)", "", 1),
	} {
		if isForcedPersistenceRefusal(status.Error(codes.FailedPrecondition, message)) {
			t.Fatalf("accepted near match %q", message)
		}
	}
	for _, code := range []codes.Code{codes.Unknown, codes.Unauthenticated, codes.PermissionDenied, codes.Canceled, codes.Internal} {
		if isForcedPersistenceRefusal(status.Error(code, sixUnresolved)) {
			t.Fatalf("accepted wrong code %v", code)
		}
	}
}

// Real RPC force-cutover, failed persistence tickets and ownership transitions;
// only disposable fixture resources are stopped by the contained adapter.
func TestForcedPersistenceContainedRecovery(t *testing.T) {
	for _, mode := range []string{"recover", "decline", "owner-change", "auth", "cancel", "exclusion-refused"} {
		t.Run(mode, func(t *testing.T) {
			ctx := reproLoopbackContext()
			a, b, c := lifecycleFixture(t)
			_, sourceEndpoint := serveCutoverFixture(t, a)
			_, endpoint := serveCutoverFixture(t, b)
			setResumeSource(b, a, sourceEndpoint)
			owner, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 6; i++ {
				ticket, e := a.AdmitRetainedWork(owner, broker.PersistenceWork)
				if e != nil {
					t.Fatal(e)
				}
				if e = ticket.PersistenceFailed(); e != nil {
					t.Fatal(e)
				}
			}
			p := &containedFixture{refuse: mode == "exclusion-refused"}
			if mode == "recover" {
				p.beforeStop = func() {
					if e := a.coexistence.owners["chat"].Close(); e != nil {
						t.Error(e)
					}
				}
			}
			b.coexistence.options.RecoverySource = p
			b.coexistence.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
				return &pb.LifecycleRecoverySelection{Identity: b.coexistence.controlIdentity(), Endpoint: endpoint, TokenFile: "fixture"}, nil
			}
			b.coexistence.options.Reload = func(context.Context, conversation.Store, *conversation.Ownership) error {
				if !p.held {
					t.Error("reload outside source exclusion")
				}
				return nil
			}
			r := resumeRequest(b, endpoint)
			r.Requester = b.coexistence.controlIdentity()
			before, err := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
			if err != nil {
				t.Fatal(err)
			}
			offer, err := b.LifecycleResume(ctx, r)
			if err != nil || !offer.GetConfirmationRequired() {
				t.Fatalf("offer: %v %v", offer, err)
			}
			after, _ := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
			if before != after || p.stop != 0 || !a.forcedCutover(owner) {
				t.Fatal("discovery changed owner/stopped source or failed to fence")
			}
			assertUnresolved := func() {
				t.Helper()
				a.coexistence.mu.Lock()
				closure := a.coexistence.closures[owner]
				a.coexistence.mu.Unlock()
				_, uncertain, e := closure.closure.Pending()
				if e != nil || uncertain != 6 {
					t.Fatalf("uncertainty discarded: %d %v", uncertain, e)
				}
			}
			assertUnresolved()
			r.RecoveryChallenge = offer.GetOriginalRecovery().GetChallenge()
			r.ConfirmOriginalRecovery = mode != "decline"
			confirmCtx := ctx
			switch mode {
			case "owner-change":
				if e := a.coexistence.owners["chat"].Release(ctx); e != nil {
					t.Fatal(e)
				}
				if _, e := c.Retain(ctx, "chat"); e != nil {
					t.Fatal(e)
				}
				before, _ = conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
			case "auth":
				b.coexistence.options.ControlAuthorize = func(context.Context) error { return status.Error(codes.Unauthenticated, "fixture auth refusal") }
			case "cancel":
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				confirmCtx = canceled
			}
			got, err := b.LifecycleResume(confirmCtx, r)
			if mode == "recover" {
				if err != nil || !got.GetReady() || !got.GetCurrentOwner() || got.GetOwner().GetHost() != "B" || got.GetOwner().GetEpoch() == owner.Epoch || p.stop != 1 {
					t.Fatalf("recovery: %v %v stops=%d", got, err, p.stop)
				}
				assertUnresolved()
				if _, e := b.LifecycleResume(ctx, r); e == nil || p.stop != 1 {
					t.Fatalf("consent replay: %v stops=%d", e, p.stop)
				}
			} else {
				if err == nil || p.stop != 0 {
					t.Fatalf("unsafe continuation: %v %v stops=%d", got, err, p.stop)
				}
				after, _ = conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
				if after != before {
					t.Fatal("refusal changed ownership")
				}
				if mode != "owner-change" {
					assertUnresolved()
				}
			}
		})
	}
}
