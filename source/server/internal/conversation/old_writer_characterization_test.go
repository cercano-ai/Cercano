package conversation

import (
	"context"
	"database/sql"
	"testing"
)

// Ownership fencing protects participating store APIs, not arbitrary old SQL.
// A successful recovery callback is deliberately NOT real death proof here.
func TestRecoveryDoesNotFenceArbitrarySQLConnection(t *testing.T) {
	ctx, path, s := ownershipFixture(t)
	old := take(t, ctx, s, "chat", "old")
	mustCoexist(t, s.EnsureConversation(WithOwnership(ctx, old), "chat", "", ""))
	raw, err := sql.Open("sqlite", path)
	mustCoexist(t, err)
	defer raw.Close()
	mustCoexist(t, raw.PingContext(ctx))
	before, err := ReadOwnership(ctx, s, "chat")
	mustCoexist(t, err)
	mustCoexist(t, old.Close())
	recovered, err := RecoverDeadOwner(ctx, s, "chat", before, "new", func(_ context.Context) error { return nil })
	mustCoexist(t, err)
	defer recovered.Close()
	refused(t, s.Rename(WithOwnership(ctx, old), "chat", "stale participating writer"))
	result, err := raw.ExecContext(ctx, "UPDATE conversations SET title=? WHERE id=?", "arbitrary old SQL still writes", "chat")
	mustCoexist(t, err)
	count, err := result.RowsAffected()
	mustCoexist(t, err)
	if count != 1 {
		t.Fatalf("updated %d rows", count)
	}
	var title string
	mustCoexist(t, raw.QueryRowContext(ctx, "SELECT title FROM conversations WHERE id='chat'").Scan(&title))
	if title != "arbitrary old SQL still writes" {
		t.Fatal(title)
	}
	after, err := ReadOwnership(ctx, s, "chat")
	mustCoexist(t, err)
	if after.Owner != "new" || after.Epoch != recovered.Epoch() {
		t.Fatal(after)
	}
}
