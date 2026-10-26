package conversation

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/chatroute"
)

func ownershipFixture(t *testing.T) (context.Context, string, *sqliteStore) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "owned.db")
	mustCoexist(t, PrepareCoexistence(ctx, path))
	store, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
	mustCoexist(t, err)
	t.Cleanup(func() { store.Close() })
	return ctx, path, store.(*sqliteStore)
}
func take(t *testing.T, ctx context.Context, s Store, id, host string) *Ownership {
	t.Helper()
	o, err := AcquireOwnership(ctx, s, id, host)
	mustCoexist(t, err)
	t.Cleanup(func() { o.Close() })
	return o
}
func refused(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected refusal")
	}
}

func TestOwnershipTransitionsReservationAndAbandonment(t *testing.T) {
	ctx, path, a := ownershipFixture(t)
	bs, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
	mustCoexist(t, err)
	defer bs.Close()
	cs, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
	mustCoexist(t, err)
	defer cs.Close()
	first := take(t, ctx, a, "c", "A-instance")
	mustCoexist(t, a.EnsureConversation(WithOwnership(ctx, first), "c", "", ""))
	_, err = AcquireOwnership(ctx, bs, "c", "B-instance")
	refused(t, err)
	sibling := take(t, ctx, a, "sibling", "A-instance")
	mustCoexist(t, a.EnsureConversation(WithOwnership(ctx, sibling), "sibling", "", ""))
	id, err := first.Handover(ctx, "B-instance")
	mustCoexist(t, err)
	pending, err := ReadOwnership(ctx, a, "c")
	mustCoexist(t, err)
	if pending.State != "reserved" || pending.Owner != "A-instance" || pending.Epoch != first.Epoch() || pending.Destination != "B-instance" || pending.Handover != id {
		t.Fatalf("%+v", pending)
	}
	for _, attempt := range []func() error{
		func() error { _, e := AcquireOwnership(ctx, cs, "c", "C-instance"); return e },
		func() error { _, e := ClaimHandover(ctx, cs, "c", "C-instance", id); return e },
		func() error { _, e := ClaimHandover(ctx, bs, "c", "B-instance", "wrong"); return e },
	} {
		refused(t, attempt())
		after, e := ReadOwnership(ctx, a, "c")
		mustCoexist(t, e)
		if after != pending {
			t.Fatal("reservation changed")
		}
	}
	mustCoexist(t, a.Append(WithOwnership(ctx, sibling), Turn{ConversationID: "sibling", Role: "user", Content: "unaffected"}))
	second, err := ClaimHandover(ctx, bs, "c", "B-instance", id)
	mustCoexist(t, err)
	defer second.Close()
	if second.Epoch() == first.Epoch() {
		t.Fatal("epoch reused")
	}
	refused(t, a.Rename(WithOwnership(ctx, first), "c", "stale"))
	mustCoexist(t, bs.Rename(WithOwnership(ctx, second), "c", "B"))
	_, err = ClaimHandover(ctx, bs, "c", "B-instance", id)
	var receipt *HandoverAlreadyClaimed
	if !errors.As(err, &receipt) || receipt.Epoch != second.Epoch() {
		t.Fatalf("retry receipt: %v", err)
	}
	mustCoexist(t, second.Release(ctx))
	third := take(t, ctx, cs, "c", "C-instance")
	if third.Epoch() == second.Epoch() {
		t.Fatal("release reused epoch")
	}
	mustCoexist(t, third.Close())
	_, err = AcquireOwnership(ctx, bs, "c", "B-instance")
	refused(t, err)
	if !strings.Contains(err.Error(), "recovery unsupported") {
		t.Fatalf("unclear abandonment diagnostic: %v", err)
	}
	files, err := os.ReadDir(path + ".ownership-locks")
	mustCoexist(t, err)
	if len(files) != 2 {
		t.Fatalf("stable locks: %d", len(files))
	}
}

func TestOwnershipEveryMutationFamily(t *testing.T) {
	ctx, path, s := ownershipFixture(t)
	o := take(t, ctx, s, "c", "A")
	valid := WithOwnership(ctx, o)
	mustCoexist(t, s.EnsureConversation(valid, "c", "", ""))
	mustCoexist(t, s.Append(valid, Turn{ID: "turn", ConversationID: "c", Role: "user", Content: "original"}))
	mustCoexist(t, s.InsertTask(valid, TaskRecord{ID: "task", ParentID: "c", Status: "accepted"}))
	run, err := s.CreateAutonomyRun(valid, AutonomyRun{ConversationID: "c", State: "running"})
	mustCoexist(t, err)
	ops := map[string]func(context.Context) error{
		"ensure":   func(c context.Context) error { return s.EnsureConversation(c, "c", "", "") },
		"subagent": func(c context.Context) error { return s.EnsureSubagentConversation(c, "child", "c", "", "", nil) },
		"rollover": func(c context.Context) error {
			return s.CreateRolledOver(c, "next", "", "", "c", Turn{Content: "handoff"})
		},
		"append": func(c context.Context) error {
			return s.Append(c, Turn{ConversationID: "c", Role: "assistant", Content: "new"})
		},
		"rename":     func(c context.Context) error { return s.Rename(c, "c", "changed") },
		"title":      func(c context.Context) error { return s.SetGeneratedTitle(c, "c", "changed") },
		"recap":      func(c context.Context) error { return s.UpdateRecap(c, "c", "changed") },
		"devdir":     func(c context.Context) error { return s.SetDevWorkDir(c, "c", "/synthetic") },
		"dismiss":    func(c context.Context) error { return s.MarkSubagentDismissed(c, "c") },
		"compaction": func(c context.Context) error { return s.SaveCompaction(c, Compaction{ConversationID: "c"}) },
		"history_usage": func(c context.Context) error {
			return s.SaveContextUsage(c, ContextUsage{ConversationID: "c", Source: "compaction"})
		},
		"request_usage": func(c context.Context) error {
			return s.SaveContextUsage(c, ContextUsage{ConversationID: "c", Source: "turn"})
		},
		"route": func(c context.Context) error {
			return s.SetChatRoute(c, "c", &chatroute.Route{Profile: "local", Model: "fixture"})
		},
		"route_clear": func(c context.Context) error { return s.SetChatRoute(c, "c", nil) },
		"autonomy_create": func(c context.Context) error {
			_, e := s.CreateAutonomyRun(c, AutonomyRun{ConversationID: "c", State: "running"})
			return e
		},
		"autonomy_update": func(c context.Context) error { return s.UpdateAutonomyRun(c, run) },
		"autonomy_compat": func(c context.Context) error { return s.SaveAutonomyRun(c, run) },
		"dispatch": func(c context.Context) error {
			return s.AppendDispatchEvent(c, DispatchEvent{ConversationID: "c", Seq: 1, Kind: "note", PayloadJSON: `{}`})
		},
		"task_insert": func(c context.Context) error {
			return s.InsertTask(c, TaskRecord{ID: "newtask", ParentID: "c", Status: "accepted"})
		},
		"task_start": func(c context.Context) error { return s.StartTask(c, "c", "task") },
		"task_finish": func(c context.Context) error {
			ok, e := s.FinishTask(c, TaskRecord{ID: "task", ParentID: "c", Status: "completed"})
			if ok {
				t.Error("rejected finish signaled success")
			}
			return e
		},
		"wake": func(c context.Context) error {
			ok, e := s.ClaimTaskWake(c, "c", "task")
			if ok {
				t.Error("rejected wake signaled success")
			}
			return e
		},
		"notification": func(c context.Context) error { return s.HandleTaskNotification(c, "c", "task") },
		"delete":       func(c context.Context) error { return s.Delete(c, "c") },
		"delete_turns": func(c context.Context) error { return s.DeleteTurns(c, "c", []string{"turn"}) },
		"prune": func(c context.Context) error {
			_, e := s.PruneRawBodies(c, "c", time.Now().Unix()+1, time.Now().Unix()+1)
			return e
		},
		"collapse": func(c context.Context) error { return s.CollapseConversation(c, "c") },
	}
	// Durable epoch mismatch with the OS lock still held exercises SQL validation,
	// rather than just the closed-capability guard after ordinary relinquishment.
	_, err = s.db.Exec(`UPDATE conversation_ownership SET epoch='new-epoch' WHERE conversation_id='c'`)
	mustCoexist(t, err)
	for name, op := range ops {
		t.Run("stale/"+name, func(t *testing.T) {
			before := coexistenceSnapshot(t, path)
			err := op(valid)
			refused(t, err)
			if !strings.Contains(err.Error(), "ownership") && !strings.Contains(err.Error(), "contract") {
				t.Fatalf("not a fence rejection: %v", err)
			}
			if !reflect.DeepEqual(before, coexistenceSnapshot(t, path)) {
				t.Fatal("stale mutation changed data")
			}
		})
	}
	_, err = s.db.Exec(`UPDATE conversation_ownership SET epoch=? WHERE conversation_id='c'`, o.Epoch())
	mustCoexist(t, err)
	for name, op := range ops {
		t.Run("unfenced/"+name, func(t *testing.T) { refused(t, op(ctx)) })
	}
	_, err = s.db.Exec(`UPDATE coexistence_metadata SET contract='changed'`)
	mustCoexist(t, err)
	for name, op := range ops {
		t.Run("contract/"+name, func(t *testing.T) {
			before := coexistenceSnapshot(t, path)
			err := op(valid)
			refused(t, err)
			if !strings.Contains(err.Error(), "ownership") && !strings.Contains(err.Error(), "contract") {
				t.Fatalf("not a fence rejection: %v", err)
			}
			if !reflect.DeepEqual(before, coexistenceSnapshot(t, path)) {
				t.Fatal("contract refusal changed data")
			}
		})
	}
	refused(t, o.Release(ctx))
	// Rejected release retained its OS lock.
	_, err = AcquireOwnership(ctx, s, "c", "other")
	refused(t, err)
	if !strings.Contains(err.Error(), "lock contention") {
		t.Fatalf("source lock lost: %v", err)
	}
}

func TestOwnershipRowFenceRolloverAndGlobalStartup(t *testing.T) {
	ctx, _, s := ownershipFixture(t)
	a := take(t, ctx, s, "a", "A")
	b := take(t, ctx, s, "b", "A")
	ca := WithOwnership(ctx, a)
	cb := WithOwnership(ctx, b)
	both := WithOwnership(ctx, a, b)
	mustCoexist(t, s.EnsureConversation(ca, "a", "", ""))
	refused(t, s.CreateRolledOver(cb, "b", "", "", "a", Turn{Content: "handoff"}))
	if _, e := s.Get(ctx, "b"); e == nil {
		t.Fatal("partial rollover survived")
	}
	mustCoexist(t, s.CreateRolledOver(both, "b", "", "", "a", Turn{Content: "handoff"}))
	refused(t, s.Rename(ca, "b", "wrong capability"))
	// Both OLD and NEW IDs are checked; a valid token cannot move another row.
	_, err := s.execOwned(ca, `UPDATE conversations SET id='a-renamed' WHERE id='a'`)
	refused(t, err)
	_, err = s.execOwned(ca, `DELETE FROM conversations`)
	refused(t, err)
	if _, e := s.Get(ctx, "a"); e != nil {
		t.Fatal("bulk mutation partially committed")
	}
	child := take(t, ctx, s, "child", "A")
	refused(t, s.EnsureSubagentConversation(WithOwnership(ctx, child), "child", "a", "", "", nil))
	mustCoexist(t, s.EnsureSubagentConversation(WithOwnership(ctx, a, child), "child", "a", "", "", nil))
	// Child writes subsequently need only the child's independent capability.
	mustCoexist(t, s.Append(WithOwnership(ctx, child), Turn{ConversationID: "child", Role: "assistant", Content: "child"}))
	refused(t, s.InitTasks(both))
	refused(t, s.ReconcileTasks(both))
	// Trigger installation survives physical connection replacement.
	s.db.SetMaxIdleConns(0)
	refused(t, s.Rename(ca, "b", "still wrong"))
	mustCoexist(t, s.Rename(cb, "b", "correct"))
}

func TestOwnershipFencedTablesComplete(t *testing.T) {
	_, _, s := ownershipFixture(t)
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	mustCoexist(t, err)
	defer rows.Close()
	for rows.Next() {
		var name string
		mustCoexist(t, rows.Scan(&name))
		switch name {
		case "coexistence_metadata", "conversation_ownership", "conversation_handovers":
			continue
		}
		if _, ok := ownedTables[name]; !ok {
			t.Errorf("unfenced table: %s", name)
		}
	}
	mustCoexist(t, rows.Err())
}

func TestOwnershipPathAliases(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fixture.db")
	if err := PrepareCoexistence(ctx, path); err != nil {
		t.Fatal(err)
	}
	a, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	o := take(t, ctx, a, "c", "A")
	alias := filepath.Join(filepath.Dir(path), "alias.db")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	b, err := OpenCoexistence(ctx, alias, CurrentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	_, err = AcquireOwnership(ctx, b, "c", "B")
	refused(t, err)
	if err := o.Release(ctx); err != nil {
		t.Fatal(err)
	}
	take(t, ctx, b, "c", "B")
	hard := filepath.Join(filepath.Dir(path), "hard.db")
	if err := os.Link(path, hard); err != nil {
		t.Fatal(err)
	}
	_, err = OpenCoexistence(ctx, hard, CurrentCoexistenceDeclaration())
	if err == nil || !strings.Contains(err.Error(), "hard-linked") {
		t.Fatalf("hard-link alias: %v", err)
	}
}

func TestOwnershipReplacementConnectionCascade(t *testing.T) {
	base, _, s := ownershipFixture(t)
	o := take(t, base, s, "c", "A")
	ctx := WithOwnership(base, o)
	mustCoexist(t, s.EnsureConversation(ctx, "c", "", ""))
	if err := s.Append(ctx, Turn{ID: "t", ConversationID: "c", Role: "user", Content: "fixture"}); err != nil {
		t.Fatal(err)
	}
	// Force the physical connection to be replaced. Neither foreign keys nor
	// the per-transaction row fences may depend on the original connection.
	s.db.SetMaxIdleConns(0)
	if err := s.Delete(ctx, "c"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM turns WHERE conversation_id='c'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("replacement connection lost cascade enforcement")
	}
}

func TestOwnershipRelationshipUpdateProbe(t *testing.T) {
	for _, col := range []string{"parent_id", "precursor_id"} {
		t.Run(col, func(t *testing.T) {
			ctx, _, s := ownershipFixture(t)
			a := take(t, ctx, s, "a", "host")
			b := take(t, ctx, s, "b", "host")
			mustCoexist(t, s.EnsureConversation(WithOwnership(ctx, a), "a", "", ""))
			mustCoexist(t, s.EnsureConversation(WithOwnership(ctx, b), "b", "", ""))
			_, err := s.execOwned(WithOwnership(ctx, a), "UPDATE conversations SET "+col+"='b' WHERE id='a'")
			refused(t, err)
		})
	}
}
func TestOwnershipTargetNoopProbe(t *testing.T) {
	for _, op := range []string{"collapse", "title"} {
		t.Run(op, func(t *testing.T) {
			ctx, _, s := ownershipFixture(t)
			a := take(t, ctx, s, "a", "host")
			b := take(t, ctx, s, "b", "host")
			mustCoexist(t, s.EnsureConversation(WithOwnership(ctx, b), "b", "", ""))
			mustCoexist(t, s.Rename(WithOwnership(ctx, b), "b", "user title"))
			if op == "collapse" {
				refused(t, s.CollapseConversation(WithOwnership(ctx, a), "b"))
			} else {
				refused(t, s.SetGeneratedTitle(WithOwnership(ctx, a), "b", "generated"))
			}
		})
	}
}

func TestOwnershipTargetedMutationAudit(t *testing.T) {
	ctx, _, s := ownershipFixture(t)
	a := take(t, ctx, s, "a", "host")
	b := take(t, ctx, s, "b", "host")
	ca, cb := WithOwnership(ctx, a), WithOwnership(ctx, b)
	mustCoexist(t, s.EnsureConversation(cb, "b", "", ""))
	mustCoexist(t, s.InsertTask(cb, TaskRecord{ID: "task", ParentID: "b", Status: "accepted"}))
	_, err := s.FinishTask(cb, TaskRecord{ID: "task", ParentID: "b", Status: "completed"})
	mustCoexist(t, err)
	mustCoexist(t, s.HandleTaskNotification(cb, "b", "task"))
	run, err := s.CreateAutonomyRun(cb, AutonomyRun{ConversationID: "b"})
	mustCoexist(t, err)
	cases := map[string]func(context.Context) error{
		"empty delete turns": func(c context.Context) error { return s.DeleteTurns(c, "b", nil) },
		"missing turn":       func(c context.Context) error { return s.DeleteTurns(c, "b", []string{"absent"}) },
		"prune":              func(c context.Context) error { _, e := s.PruneRawBodies(c, "b", 1, 1); return e },
		"dismiss main":       func(c context.Context) error { return s.MarkSubagentDismissed(c, "b") },
		"clear absent route": func(c context.Context) error { return s.SetChatRoute(c, "b", nil) },
		"duplicate finish": func(c context.Context) error {
			_, e := s.FinishTask(c, TaskRecord{ID: "task", ParentID: "b", Status: "completed"})
			return e
		},
		"claim no notification":  func(c context.Context) error { _, e := s.ClaimTaskWake(c, "b", "task"); return e },
		"handle no notification": func(c context.Context) error { return s.HandleTaskNotification(c, "b", "task") },
		"run id only":            func(c context.Context) error { return s.UpdateAutonomyRun(c, AutonomyRun{RunID: run.RunID}) },
	}
	for name, op := range cases {
		t.Run(name, func(t *testing.T) { refused(t, op(ca)); mustCoexist(t, op(cb)) })
	}
	// Missing direct IDs retain idempotence only with a live capability for that ID.
	missing := take(t, ctx, s, "missing", "host")
	refused(t, s.CollapseConversation(ca, "missing"))
	mustCoexist(t, s.CollapseConversation(WithOwnership(ctx, missing), "missing"))
	mustCoexist(t, missing.Release(ctx))
	refused(t, s.CollapseConversation(WithOwnership(ctx, missing), "missing"))
	if err := s.HandleTaskNotification(cb, "b", "absent"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing task: %v", err)
	}
	if err := s.UpdateAutonomyRun(cb, AutonomyRun{RunID: "absent"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing run: %v", err)
	}
	refused(t, s.HandleTaskNotification(ca, "a", "task"))
	refused(t, s.UpdateAutonomyRun(ca, AutonomyRun{RunID: run.RunID, ConversationID: "a"}))
}

func TestOwnershipRelationshipUpdateOldAndNew(t *testing.T) {
	for _, col := range []string{"parent_id", "precursor_id"} {
		t.Run(col, func(t *testing.T) {
			ctx, _, s := ownershipFixture(t)
			a := take(t, ctx, s, "a", "host")
			b := take(t, ctx, s, "b", "host")
			c := take(t, ctx, s, "c", "host")
			all := WithOwnership(ctx, a, b, c)
			for _, id := range []string{"a", "b", "c"} {
				mustCoexist(t, s.EnsureConversation(all, id, "", ""))
			}
			_, err := s.execOwned(all, "UPDATE conversations SET "+col+"='b' WHERE id='a'")
			mustCoexist(t, err)
			_, err = s.execOwned(WithOwnership(ctx, a, c), "UPDATE conversations SET "+col+"='c' WHERE id='a'")
			refused(t, err)
			_, err = s.execOwned(WithOwnership(ctx, a), "UPDATE conversations SET "+col+"='' WHERE id='a'")
			refused(t, err)
			_, err = s.execOwned(all, "UPDATE conversations SET "+col+"='c' WHERE id='a'")
			mustCoexist(t, err)
			// Ordinary content updates do not change relationship authority.
			mustCoexist(t, s.UpdateRecap(WithOwnership(ctx, a), "a", "recap"))
		})
	}
}
