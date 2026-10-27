package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"modernc.org/sqlite"
)

// Two store handles model independent hosts; different IDs do not contend on
// ownership file locks. Hold a fenced write at its read snapshot, then attempt
// the other host's ownership transaction before allowing the first to commit.
func TestCoexistenceIndependentWriterAdmission(t *testing.T) {
	ctx, path, a := ownershipFixture(t)
	bs, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
	mustCoexist(t, err)
	defer bs.Close()
	b := bs.(*sqliteStore)
	owner := take(t, ctx, a, "first", "A")
	mustCoexist(t, a.EnsureConversation(WithOwnership(ctx, owner), "first", "", ""))
	tx, err := a.beginOwned(WithOwnership(ctx, owner), nil, "first")
	mustCoexist(t, err)
	defer tx.Rollback()
	// A bounded busy timeout makes this admission assertion deterministic: a
	// deferred BEGIN would succeed, even though it cannot safely upgrade later.
	_, err = b.db.ExecContext(ctx, "PRAGMA busy_timeout=1")
	mustCoexist(t, err)
	other, err := b.beginWrite(ctx, nil)
	if err == nil {
		other.Rollback()
		t.Fatal("second writer admitted while first holds its snapshot")
	}
	var busy *sqlite.Error
	if !errors.As(err, &busy) || busy.Code() != 5 {
		t.Fatalf("want SQLITE_BUSY admission refusal, got %v", err)
	}
	_, err = b.db.ExecContext(ctx, "PRAGMA busy_timeout=5000")
	mustCoexist(t, err)
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		o, e := AcquireOwnership(ctx, b, "independent", "B")
		if o != nil {
			defer o.Close()
		}
		done <- e
	}()
	<-started
	_, err = tx.ExecContext(ctx, `UPDATE conversations SET title='written' WHERE id='first'`)
	mustCoexist(t, err)
	mustCoexist(t, tx.Commit())
	select {
	case err = <-done:
		mustCoexist(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("independent writer did not finish")
	}
	state, err := ReadOwnership(ctx, b, "independent")
	mustCoexist(t, err)
	if state.Owner != "B" || state.State != "owned" {
		t.Fatalf("unexpected independent ownership: %+v", state)
	}
	// Replacement connections retain the admission policy and cancellation does
	// not leave a transaction/lock behind.
	b.db.SetMaxIdleConns(0)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = b.beginWrite(cancelled, nil)
	if err != context.Canceled {
		t.Fatalf("cancelled begin: %v", err)
	}
	tx, err = a.db.BeginTx(ctx, nil)
	mustCoexist(t, err)
	defer tx.Rollback()
	short, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stop()
	other, err = b.beginWrite(short, nil)
	if err == nil {
		other.Rollback()
		t.Fatal("replacement connection bypassed admission")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired admission: %v", err)
	}
	mustCoexist(t, tx.Rollback())
	o, err := AcquireOwnership(ctx, b, "after-cancel", "B")
	mustCoexist(t, err)
	defer o.Close()
}
