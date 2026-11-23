package conversation

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"cercano/source/server/internal/chatroute"
)

func TestMutationFailureBoundaries(t *testing.T) {
	ctx, _, s := ownershipFixture(t)
	owner := take(t, ctx, s, "c", "host")
	ctx = WithOwnership(ctx, owner)
	mustCoexist(t, s.EnsureConversation(ctx, "c", "", ""))
	for name, err := range map[string]error{
		"validation":    s.SetChatRoute(ctx, "c", &chatroute.Route{}),
		"lookup":        s.SetChatRoute(ctx, "missing", &chatroute.Route{Profile: "p", Model: "m"}),
		"authorization": s.SetChatRoute(context.Background(), "c", nil),
	} {
		if err == nil || IsMutationFailure(err) {
			t.Fatalf("%s classified as mutation: %v", name, err)
		}
	}
	// Relationship checks happen in SQL triggers, not just authorizeTarget.
	_, err := s.execOwned(ctx, `INSERT INTO conversations(id, parent_id, started_at, last_turn_at) VALUES ('child', 'unowned', 0, 0)`)
	if err == nil || IsMutationFailure(err) {
		t.Fatalf("trigger authorization classified as mutation: %v", err)
	}
	_, err = s.execTarget(ctx, "c", `INSERT INTO missing_table VALUES (1)`)
	if !IsMutationFailure(err) {
		t.Fatalf("statement failure unmarked: %v", err)
	}
	sentinel := errors.New("write")
	if !errors.Is(&MutationError{Err: sentinel}, sentinel) {
		t.Fatal("lost error identity")
	}
}

func TestMutationFailureCommit(t *testing.T) {
	for _, targeted := range []bool{false, true} {
		t.Run(map[bool]string{false: "bulk", true: "targeted"}[targeted], func(t *testing.T) {
			ctx, _, s := ownershipFixture(t)
			ctx = WithOwnership(ctx, take(t, ctx, s, "c", "host"))
			mustCoexist(t, s.EnsureConversation(ctx, "c", "", ""))
			// A deferred constraint succeeds at Exec and fails only at Commit.
			_, err := s.db.ExecContext(ctx, `CREATE TABLE commit_probe (id INTEGER PRIMARY KEY, parent INTEGER REFERENCES commit_probe(id) DEFERRABLE INITIALLY DEFERRED)`)
			mustCoexist(t, err)
			tx, err := s.beginOwned(ctx, nil)
			mustCoexist(t, err)
			_, err = tx.ExecContext(ctx, `INSERT INTO commit_probe VALUES (1, 2)`)
			mustCoexist(t, err)
			err = tx.Commit()
			if err == nil {
				t.Fatal("probe did not reach failing commit")
			}
			var result sql.Result
			if targeted {
				result, err = s.execTarget(ctx, "c", `INSERT INTO commit_probe VALUES (1, 2)`)
			} else {
				result, err = s.execOwned(ctx, `INSERT INTO commit_probe VALUES (1, 2)`)
			}
			if result != nil || !IsMutationFailure(err) {
				t.Fatalf("commit failure unmarked: %v %v", result, err)
			}
		})
	}
}
