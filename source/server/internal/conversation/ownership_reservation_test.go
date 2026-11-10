package conversation

import (
	"errors"
	"testing"
	"time"
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

func TestOwnershipSourceQualifiedClaim(t *testing.T) {
	ctx, _, store := ownershipFixture(t)
	a := take(t, ctx, store, "one", "A")
	mustCoexist(t, a.ReserveHandover(ctx, "B", "accepted"))
	before, e := ReadOwnership(ctx, store, "one")
	mustCoexist(t, e)
	for _, source := range [][2]string{{"wrong", a.Epoch()}, {"A", "wrong"}, {"", a.Epoch()}, {"A", ""}} {
		_, e = ClaimHandoverFrom(ctx, store, "one", "B", "accepted", source[0], source[1])
		refused(t, e)
		after, e := ReadOwnership(ctx, store, "one")
		mustCoexist(t, e)
		if after != before {
			t.Fatal("rejected claim mutated reservation")
		}
	}
	b, e := ClaimHandoverFrom(ctx, store, "one", "B", "accepted", "A", a.Epoch())
	mustCoexist(t, e)
	mustCoexist(t, b.Release(ctx))
	// A historical receipt alone cannot establish original provenance.
	_, e = ClaimHandoverFrom(ctx, store, "one", "B", "accepted", "A", a.Epoch())
	refused(t, e)
}

func TestOwnershipReservationWaitsForWriter(t *testing.T) {
	ctx, _, s := ownershipFixture(t)
	a := take(t, ctx, s, "one", "A")
	valid := WithOwnership(ctx, a)
	mustCoexist(t, s.EnsureConversation(valid, "one", "", ""))
	tx, err := s.beginOwned(valid, nil, "one")
	mustCoexist(t, err)
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE conversations SET title='pending' WHERE id='one'`)
	mustCoexist(t, err)
	done := make(chan error, 1)
	go func() { done <- a.ReserveHandover(ctx, "C", "new") }()
	select {
	case err := <-done:
		t.Fatalf("cutover passed pending writer: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	mustCoexist(t, tx.Commit())
	select {
	case err := <-done:
		mustCoexist(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("cutover blocked")
	}
	refused(t, s.Rename(valid, "one", "stale"))
	var title string
	mustCoexist(t, s.db.QueryRow(`SELECT title FROM conversations WHERE id='one'`).Scan(&title))
	if title != "pending" {
		t.Fatal(title)
	}
}

// Pending and completed operation IDs cannot authorize a new reservation,
// including after reacquiring the original conversation at a fresh epoch.
func TestOwnershipReservationRejectsReusedOperation(t *testing.T) {
	ctx, _, store := ownershipFixture(t)
	a := take(t, ctx, store, "one", "A")
	other := take(t, ctx, store, "other", "X")
	mustCoexist(t, a.ReserveHandover(ctx, "B", "operation"))
	if a.usable.Load() || a.lock != nil {
		t.Fatal("source still usable or locked")
	}
	before, err := ReadOwnership(ctx, store, "one")
	mustCoexist(t, err)
	refused(t, a.ReserveHandover(ctx, "C", "operation"))
	refused(t, other.ReserveHandover(ctx, "Y", "operation"))
	after, err := ReadOwnership(ctx, store, "one")
	mustCoexist(t, err)
	if after != before {
		t.Fatal("rejected reservation mutated row")
	}
	b, err := ClaimHandoverFrom(ctx, store, "one", "B", "operation", "A", a.Epoch())
	mustCoexist(t, err)
	mustCoexist(t, b.Release(ctx))
	fresh := take(t, ctx, store, "one", "A")
	before, err = ReadOwnership(ctx, store, "one")
	mustCoexist(t, err)
	refused(t, fresh.ReserveHandover(ctx, "C", "operation"))
	refused(t, other.ReserveHandover(ctx, "Y", "operation"))
	refused(t, a.ReserveHandover(ctx, "B", "operation"))
	after, err = ReadOwnership(ctx, store, "one")
	mustCoexist(t, err)
	if after != before {
		t.Fatal("historical operation mutated ownership")
	}
}
