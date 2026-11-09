package server

import (
	"context"
	"google.golang.org/grpc/peer"
	"net"
	"testing"

	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Exercise the control directly: no listeners, peers, or production stores.
func TestLifecycleClaimHistoricalReceiptIsNotOwnership(t *testing.T) {
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}})
	a, b, c := lifecycleFixture(t)
	b.coexistence.options.ControlAuthorize = func(context.Context) error { return nil }
	reloads := 0
	b.coexistence.options.Reload = func(context.Context, conversation.Store, *conversation.Ownership) error {
		reloads++
		return nil
	}
	source, err := a.Retain(ctx, "history")
	if err != nil {
		t.Fatal(err)
	}
	acceptance, err := b.AcceptIncoming(source, "historical-claim", "build-A", conversation.CurrentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	if err = a.HandoverRetained(ctx, source, nil, acceptance); err != nil {
		t.Fatal(err)
	}
	r := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), Owner: wireOwner(source), HandoverId: acceptance.h.HandoverID, Receipt: acceptance.receipt, Management: true}
	first, err := b.LifecycleClaim(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := b.LifecycleClaim(ctx, r)
	if err != nil || retry.GetOwner().GetEpoch() != first.Owner.Epoch {
		t.Fatalf("current retry: %v %v", retry, err)
	}
	if err = b.ReleaseRetained(ctx, controlOwner(&pb.LifecycleControlRequest{Owner: first.Owner}), nil); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"released", "reowned"} {
		if phase == "reowned" {
			if _, err = c.Retain(ctx, "history"); err != nil {
				t.Fatal(err)
			}
		}
		before, err := conversation.ReadOwnership(ctx, b.coexistence.store, "history")
		if err != nil {
			t.Fatal(err)
		}
		got, err := b.LifecycleClaim(ctx, r)
		if status.Code(err) != codes.FailedPrecondition || got != nil {
			t.Fatalf("%s: historical receipt reported as ownership: %v %v", phase, got, err)
		}
		after, err := conversation.ReadOwnership(ctx, b.coexistence.store, "history")
		if err != nil || before != after {
			t.Fatalf("historical retry mutated ownership: %v", err)
		}
		epoch, err := conversation.HandoverResult(ctx, b.coexistence.store, "history", "B", "historical-claim")
		if err != nil || epoch != first.Owner.Epoch {
			t.Fatalf("lost historical evidence: %q %v", epoch, err)
		}
		if reloads != 1 {
			t.Fatalf("claim retry reloaded historical execution: %d reloads", reloads)
		}
	}
}
