package conversation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func developmentOwnershipFixture(t *testing.T) (context.Context, string, *sqliteStore) {
	t.Helper()
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	mustCoexist(t, err)
	mustCoexist(t, os.Chmod(root, 0700))
	path := filepath.Join(root, "development.db")
	mustCoexist(t, PrepareDevelopmentStore(ctx, path))
	store, err := OpenCoexistence(ctx, path, DevelopmentCoexistenceDeclaration())
	mustCoexist(t, err)
	t.Cleanup(func() { store.Close() })
	return ctx, path, store.(*sqliteStore)
}

func TestDevelopmentOwnershipLifecycle(t *testing.T) {
	ctx, path, a := developmentOwnershipFixture(t)
	b, err := OpenCoexistence(ctx, path, DevelopmentCoexistenceDeclaration())
	mustCoexist(t, err)
	defer b.Close()
	first := take(t, ctx, a, "c", "A")
	mustCoexist(t, a.EnsureConversation(WithOwnership(ctx, first), "c", "", ""))
	mustCoexist(t, a.Append(WithOwnership(ctx, first), Turn{ConversationID: "c", Role: "user", Content: "before release"}))
	mustCoexist(t, first.Release(ctx))
	refused(t, a.Rename(WithOwnership(ctx, first), "c", "stale"))
	second := take(t, ctx, a, "c", "A")
	if second.Epoch() == first.Epoch() {
		t.Fatal("release reused epoch")
	}
	mustCoexist(t, a.Rename(WithOwnership(ctx, second), "c", "reacquired"))
	id, err := second.Handover(ctx, "B")
	mustCoexist(t, err)
	refused(t, a.Rename(WithOwnership(ctx, second), "c", "stale"))
	third, err := ClaimHandover(ctx, b, "c", "B", id)
	mustCoexist(t, err)
	defer third.Close()
	if third.Epoch() == second.Epoch() {
		t.Fatal("handover reused epoch")
	}
	mustCoexist(t, b.Append(WithOwnership(ctx, third), Turn{ConversationID: "c", Role: "assistant", Content: "after handover"}))
	epoch, err := HandoverResult(ctx, b, "c", "B", id)
	mustCoexist(t, err)
	if epoch != third.Epoch() {
		t.Fatal("wrong handover receipt")
	}
	mustCoexist(t, third.Release(ctx))
}

func TestOwnershipDeclarationMismatchFailsClosed(t *testing.T) {
	for _, development := range []bool{false, true} {
		name := "fixture"
		if development {
			name = "development"
		}
		for _, operation := range []string{"acquire", "write", "release", "handover", "claim"} {
			t.Run(name+"/"+operation, func(t *testing.T) {
				var ctx context.Context
				var path string
				var s *sqliteStore
				wrong := DevelopmentCoexistenceDeclaration()
				if development {
					ctx, path, s = developmentOwnershipFixture(t)
					wrong = CurrentCoexistenceDeclaration()
				} else {
					ctx, path, s = ownershipFixture(t)
				}
				// Neither preflight nor open may substitute the other same-schema declaration.
				refused(t, PreflightCoexistence(ctx, path, wrong))
				other, err := OpenCoexistence(ctx, path, wrong)
				if other != nil {
					other.Close()
				}
				refused(t, err)
				owner := take(t, ctx, s, "c", "A")
				mustCoexist(t, s.EnsureConversation(WithOwnership(ctx, owner), "c", "", ""))
				var handover string
				if operation == "claim" {
					handover, err = owner.Handover(ctx, "B")
					mustCoexist(t, err)
				}
				before, err := ReadOwnership(ctx, s, "c")
				mustCoexist(t, err)
				var titleBefore string
				mustCoexist(t, s.db.QueryRowContext(ctx, `SELECT title FROM conversations WHERE id='c'`).Scan(&titleBefore))
				// Tamper after validation: transactions must retain the original expectation,
				// not accept either known contract or learn the new label from the database.
				_, err = s.db.ExecContext(ctx, `UPDATE coexistence_metadata SET contract=?`, wrong.Contract)
				mustCoexist(t, err)
				switch operation {
				case "acquire":
					var acquired *Ownership
					acquired, err = AcquireOwnership(ctx, s, "new", "B")
					if acquired != nil {
						acquired.Close()
					}
				case "write":
					err = s.Rename(WithOwnership(ctx, owner), "c", "must not persist")
				case "release":
					err = owner.Release(ctx)
				case "handover":
					_, err = owner.Handover(ctx, "B")
				case "claim":
					var claimed *Ownership
					claimed, err = ClaimHandover(ctx, s, "c", "B", handover)
					if claimed != nil {
						claimed.Close()
					}
				}
				if err == nil || !strings.Contains(err.Error(), "coexistence contract mismatch") {
					t.Fatalf("expected exact-contract refusal, got %v", err)
				}
				after, err := ReadOwnership(ctx, s, "c")
				mustCoexist(t, err)
				if after != before {
					t.Fatal("refused transaction changed ownership")
				}
				var titleAfter string
				mustCoexist(t, s.db.QueryRowContext(ctx, `SELECT title FROM conversations WHERE id='c'`).Scan(&titleAfter))
				if titleAfter != titleBefore {
					t.Fatal("refused write changed conversation")
				}
				var added int
				mustCoexist(t, s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM conversation_ownership WHERE conversation_id='new') + (SELECT count(*) FROM conversation_handovers)`).Scan(&added))
				if added != 0 {
					t.Fatal("refused acquisition or claim persisted")
				}
				// Restoring the validated declaration proves refusal did not consume
				// the active capability or mutate the conversation.
				_, err = s.db.ExecContext(ctx, `UPDATE coexistence_metadata SET contract=?`, s.coexistenceDeclaration.Contract)
				mustCoexist(t, err)
				if operation != "claim" {
					mustCoexist(t, owner.Release(ctx))
				}
			})
		}
	}
}
