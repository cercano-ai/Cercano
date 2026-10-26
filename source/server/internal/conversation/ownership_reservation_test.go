package conversation

import (
	"errors"
	"testing"
)

func TestOwnershipExplicitReservation(t *testing.T) {
	ctx, _, store := ownershipFixture(t)
	a := take(t, ctx, store, "one", "A")
	refused(t, a.ReserveHandover(ctx, "B", ""))
	refused(t, a.ReserveHandover(ctx, "A", "accepted"))
	mustCoexist(t, a.ReserveHandover(ctx, "B", "accepted"))
	state, e := ReadOwnership(ctx, store, "one")
	mustCoexist(t, e)
	if state.State != "reserved" || state.Handover != "accepted" || state.Epoch != a.Epoch() {
		t.Fatal(state)
	}
	_, e = ClaimHandover(ctx, store, "one", "C", "accepted")
	refused(t, e)
	_, e = ClaimHandover(ctx, store, "one", "B", "wrong")
	refused(t, e)
	b, e := ClaimHandover(ctx, store, "one", "B", "accepted")
	mustCoexist(t, e)
	defer b.Close()
	if b.Epoch() == a.Epoch() {
		t.Fatal("epoch reused")
	}
	_, e = ClaimHandover(ctx, store, "one", "B", "accepted")
	var receipt *HandoverAlreadyClaimed
	if !errors.As(e, &receipt) || receipt.Epoch != b.Epoch() {
		t.Fatal(e)
	}
	other := take(t, ctx, store, "other", "A")
	refused(t, other.ReserveHandover(ctx, "B", "accepted"))
	mustCoexist(t, other.Release(ctx))
}
