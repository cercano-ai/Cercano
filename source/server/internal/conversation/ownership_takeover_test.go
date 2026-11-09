package conversation

import (
	"strings"
	"testing"
	"time"
)

func TestReserveTakeoverOwnedAndRetry(t *testing.T) {
	ctx, path, s := ownershipFixture(t)
	peer, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
	mustCoexist(t, err)
	defer peer.Close()
	a := take(t, ctx, s, "one", "A")
	mustCoexist(t, s.EnsureConversation(WithOwnership(ctx, a), "one", "", ""))
	mustCoexist(t, a.ReserveTakeover(ctx, "C", "new"))
	if a.usable.Load() || a.lock != nil {
		t.Fatal("source still usable or locked")
	}
	mustCoexist(t, a.ReserveTakeover(ctx, "C", "new"))
	refused(t, a.ReserveTakeover(ctx, "D", "new")) // same ID, different destination
	row, err := ReadOwnership(ctx, s, "one")
	mustCoexist(t, err)
	if row.State != "reserved" || row.Owner != "A" || row.Epoch != a.Epoch() || row.Destination != "C" || row.Handover != "new" {
		t.Fatal(row)
	}
	refused(t, s.Rename(WithOwnership(ctx, a), "one", "stale"))
	c, err := ClaimHandoverFrom(ctx, peer, "one", "C", "new", "A", a.Epoch())
	mustCoexist(t, err)
	mustCoexist(t, c.Release(ctx))
	before, err := ReadOwnership(ctx, s, "one")
	mustCoexist(t, err)
	refused(t, a.ReserveTakeover(ctx, "C", "new")) // historical receipt is not authority
	after, err := ReadOwnership(ctx, s, "one")
	mustCoexist(t, err)
	if after != before {
		t.Fatal("historical retry mutated ownership")
	}
}

func TestReserveTakeoverReservedAndCompetingClaim(t *testing.T) {
	for _, claimFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "takeover-wins", true: "claim-wins"}[claimFirst], func(t *testing.T) {
			ctx, path, s := ownershipFixture(t)
			peer, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
			mustCoexist(t, err)
			defer peer.Close()
			a := take(t, ctx, s, "one", "A")
			mustCoexist(t, a.ReserveHandover(ctx, "B", "old"))
			if claimFirst {
				b, err := ClaimHandover(ctx, peer, "one", "B", "old")
				mustCoexist(t, err)
				// Release its lock but retain the competing owned epoch, forcing a durable recheck.
				refused(t, a.ReserveTakeover(ctx, "C", "new")) // nonblocking lock contention
				mustCoexist(t, b.Close())
				before, err := ReadOwnership(ctx, s, "one")
				mustCoexist(t, err)
				err = a.ReserveTakeover(ctx, "C", "new")
				refused(t, err)
				if !strings.Contains(err.Error(), "stale") {
					t.Fatal(err)
				}
				after, err := ReadOwnership(ctx, s, "one")
				mustCoexist(t, err)
				if before != after {
					t.Fatal("stale takeover mutated row")
				}
			} else {
				mustCoexist(t, a.ReserveTakeover(ctx, "C", "new"))
				mustCoexist(t, a.ReserveTakeover(ctx, "C", "new"))
				_, err := ClaimHandover(ctx, peer, "one", "B", "old")
				refused(t, err)
				c, err := ClaimHandoverFrom(ctx, peer, "one", "C", "new", "A", a.Epoch())
				mustCoexist(t, err)
				defer c.Close()
			}
		})
	}
}

func TestReserveTakeoverConcurrentOldClaim(t *testing.T) {
	for i := 0; i < 10; i++ {
		ctx, path, s := ownershipFixture(t)
		peer, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
		mustCoexist(t, err)
		a := take(t, ctx, s, "one", "A")
		mustCoexist(t, a.ReserveHandover(ctx, "B", "old"))
		start := make(chan struct{})
		done := make(chan error, 1)
		go func() { <-start; done <- a.ReserveTakeover(ctx, "C", "new") }()
		close(start)
		b, claimErr := ClaimHandoverFrom(ctx, peer, "one", "B", "old", "A", a.Epoch())
		takeoverErr := <-done
		if (claimErr == nil) == (takeoverErr == nil) {
			t.Fatalf("expected exactly one winner: claim=%v takeover=%v", claimErr, takeoverErr)
		}
		row, err := ReadOwnership(ctx, s, "one")
		mustCoexist(t, err)
		if claimErr == nil {
			if row.State != "owned" || row.Epoch != b.Epoch() {
				t.Fatal(row)
			}
			mustCoexist(t, b.Close())
			refused(t, a.ReserveTakeover(ctx, "C", "new"))
		} else if row.State != "reserved" || row.Handover != "new" || row.Epoch != a.Epoch() {
			t.Fatal(row)
		}
		mustCoexist(t, peer.Close())
	}
}

func TestReserveTakeoverWaitsForWriter(t *testing.T) {
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
	go func() { done <- a.ReserveTakeover(ctx, "C", "new") }()
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

func TestReserveTakeoverInvalidAndHistory(t *testing.T) {
	ctx, _, s := ownershipFixture(t)
	a := take(t, ctx, s, "one", "A")
	for _, args := range [][2]string{{"", "op"}, {"A", "op"}, {"B", ""}} {
		refused(t, a.ReserveTakeover(ctx, args[0], args[1]))
	}
	other := take(t, ctx, s, "other", "X")
	mustCoexist(t, other.ReserveHandover(ctx, "Y", "pending"))
	refused(t, a.ReserveTakeover(ctx, "B", "pending"))
	b, err := ClaimHandover(ctx, s, "other", "Y", "pending")
	mustCoexist(t, err)
	mustCoexist(t, b.Release(ctx))
	refused(t, a.ReserveTakeover(ctx, "B", "pending"))
	mustCoexist(t, a.Release(ctx))
	refused(t, a.ReserveTakeover(ctx, "B", "fresh"))
	// Completed IDs are forbidden even on the original conversation with a
	// newly acquired source epoch; receipts cannot authorize a reservation.
	x := take(t, ctx, s, "other", "X")
	refused(t, x.ReserveTakeover(ctx, "Z", "pending"))
	for _, bad := range []*Ownership{nil, {}, {store: s, conversation: "other", owner: "", epoch: x.Epoch()}, {store: s, conversation: "other", owner: "X", epoch: ""}} {
		refused(t, bad.ReserveTakeover(ctx, "Z", "fresh"))
	}
	closed := take(t, ctx, s, "closed", "X")
	mustCoexist(t, closed.Close())
	refused(t, closed.ReserveTakeover(ctx, "Z", "fresh"))
}
