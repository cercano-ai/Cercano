package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDeadOwnerRecovery(t *testing.T) {
	ctx, _, s := ownershipFixture(t)
	root := take(t, ctx, s, "root", "A")
	child := take(t, ctx, s, "child", "A")
	other := take(t, ctx, s, "other", "A")
	c := WithOwnership(ctx, root, child, other)
	mustCoexist(t, s.EnsureConversation(c, "root", "", ""))
	mustCoexist(t, s.EnsureSubagentConversation(c, "child", "root", "", "", nil))
	mustCoexist(t, s.EnsureConversation(c, "other", "", ""))
	mustCoexist(t, s.Append(c, Turn{ConversationID: "root", Role: "user", Content: "preserve me"}))
	mustCoexist(t, s.InsertTask(c, TaskRecord{ID: "task", ParentID: "root", ChildID: "child", Status: "accepted"}))
	mustCoexist(t, s.StartTask(c, "root", "task"))
	mustCoexist(t, s.InsertTask(c, TaskRecord{ID: "nested", ParentID: "child", ChildID: "never-materialized", Status: "accepted"}))
	mustCoexist(t, s.InsertTask(c, TaskRecord{ID: "sibling-task", ParentID: "other", Status: "accepted"}))
	_, err := s.CreateAutonomyRun(c, AutonomyRun{ConversationID: "root", State: "running"})
	mustCoexist(t, err)
	v, err := ReadOwnership(ctx, s, "root")
	mustCoexist(t, err)
	// An available endpoint is irrelevant: a held lock always prevents recovery.
	_, err = RecoverDeadOwner(ctx, s, "root", v, "B", func(context.Context) error { return nil })
	refused(t, err)
	mustCoexist(t, root.Close())
	mustCoexist(t, child.Close())
	_, err = RecoverDeadOwner(ctx, s, "root", v, "B", func(context.Context) error { return errors.New("unreachable is not dead") })
	refused(t, err)
	after, err := ReadOwnership(ctx, s, "root")
	mustCoexist(t, err)
	if after != v {
		t.Fatal("refusal mutated owner")
	}
	recovered, err := RecoverDeadOwner(ctx, s, "root", v, "B", func(context.Context) error { return nil })
	mustCoexist(t, err)
	defer recovered.Close()
	if recovered.Epoch() == v.Epoch {
		t.Fatal("epoch reused")
	}
	mustCoexist(t, s.Append(WithOwnership(ctx, recovered), Turn{ConversationID: "root", Role: "assistant", Content: "new owner"}))
	for _, x := range []struct{ parent, id string }{{"root", "task"}, {"child", "nested"}} {
		task, e := s.GetTask(ctx, x.parent, x.id)
		mustCoexist(t, e)
		if task.Status != "interrupted" || !task.WakeClaimed || !task.NotificationPending || !strings.Contains(task.Error, "Not replayed") {
			t.Fatalf("bad recovery: %+v", task)
		}
	}
	sibling, err := s.GetTask(ctx, "other", "sibling-task")
	mustCoexist(t, err)
	if sibling.Status != "accepted" {
		t.Fatal("changed unrelated task")
	}
	cs, err := ReadOwnership(ctx, s, "child")
	mustCoexist(t, err)
	if cs.State != "released" {
		t.Fatal(cs)
	}
	run, err := s.GetActiveAutonomyRun(ctx, "root")
	mustCoexist(t, err)
	if run.BlockerJSON == "" || run.State != "running" {
		t.Fatal("autonomy not paused using blocker")
	}
	_, err = AcquireOwnership(ctx, s, "root", "C")
	refused(t, err)
	receipt, err := HandoverResult(ctx, s, "root", "B", RecoveryID("root", v, "B"))
	mustCoexist(t, err)
	if receipt != recovered.Epoch() {
		t.Fatal("receipt epoch")
	}
	mustCoexist(t, s.Rename(WithOwnership(ctx, other), "other", "still usable"))
}

func TestDeadOwnerRecoveryRejectsStaleAndReserved(t *testing.T) {
	for _, mode := range []string{"stale", "reserved", "foreign-child", "locked-child"} {
		t.Run(mode, func(t *testing.T) {
			ctx, _, s := ownershipFixture(t)
			o := take(t, ctx, s, "c", "A")
			mustCoexist(t, s.EnsureConversation(WithOwnership(ctx, o), "c", "", ""))
			v, err := ReadOwnership(ctx, s, "c")
			mustCoexist(t, err)
			if mode == "foreign-child" || mode == "locked-child" {
				host := "A"
				if mode == "foreign-child" {
					host = "C"
				}
				ch := take(t, ctx, s, "kid", host)
				mustCoexist(t, s.EnsureSubagentConversation(WithOwnership(ctx, o, ch), "kid", "c", "", "", nil))
				if mode == "foreign-child" {
					mustCoexist(t, ch.Close())
				}
			}
			if mode == "reserved" {
				mustCoexist(t, o.ReserveHandover(ctx, "B", "pending"))
			} else {
				mustCoexist(t, o.Close())
			}
			if mode == "stale" {
				v.Epoch = "wrong"
			}
			before, err := ReadOwnership(ctx, s, "c")
			mustCoexist(t, err)
			_, err = RecoverDeadOwner(ctx, s, "c", v, "B", func(context.Context) error { return nil })
			refused(t, err)
			after, err := ReadOwnership(ctx, s, "c")
			mustCoexist(t, err)
			if after != before {
				t.Fatal("refusal changed durable state")
			}
		})
	}
}

func TestDeadOwnerRecoveryAtomicFailure(t *testing.T) {
	ctx, _, s := ownershipFixture(t)
	o := take(t, ctx, s, "c", "A")
	c := WithOwnership(ctx, o)
	mustCoexist(t, s.EnsureConversation(c, "c", "", ""))
	mustCoexist(t, s.InsertTask(c, TaskRecord{ID: "t", ParentID: "c", Status: "accepted"}))
	v, err := ReadOwnership(ctx, s, "c")
	mustCoexist(t, err)
	mustCoexist(t, o.Close())
	_, err = s.db.Exec(`CREATE TRIGGER reject_recovery BEFORE UPDATE ON conversation_ownership WHEN NEW.owner='B' BEGIN SELECT RAISE(ABORT,'injected'); END`)
	mustCoexist(t, err)
	_, err = RecoverDeadOwner(ctx, s, "c", v, "B", func(context.Context) error { return nil })
	refused(t, err)
	task, err := s.GetTask(ctx, "c", "t")
	mustCoexist(t, err)
	if task.Status != "accepted" {
		t.Fatal("partial task mutation")
	}
	after, err := ReadOwnership(ctx, s, "c")
	mustCoexist(t, err)
	if after != v {
		t.Fatal("partial ownership mutation")
	}
	_, err = s.db.Exec(`DROP TRIGGER reject_recovery`)
	mustCoexist(t, err)
	recovered, err := RecoverDeadOwner(ctx, s, "c", v, "B", func(context.Context) error { return nil })
	mustCoexist(t, err)
	defer recovered.Close()
}

func TestReservedDeadOwnerRecovery(t *testing.T) {
	for _, claimFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(claimFirst), func(t *testing.T) {
			ctx, _, s := ownershipFixture(t)
			o := take(t, ctx, s, "c", "A")
			c := WithOwnership(ctx, o)
			mustCoexist(t, s.EnsureConversation(c, "c", "", ""))
			mustCoexist(t, s.Append(c, Turn{ConversationID: "c", Role: "user", Content: "committed"}))
			mustCoexist(t, s.InsertTask(c, TaskRecord{ID: "t", ParentID: "c", Status: "accepted"}))
			mustCoexist(t, o.ReserveHandover(ctx, "B", "old-operation"))
			v, err := ReadOwnership(ctx, s, "c")
			mustCoexist(t, err)
			if claimFirst {
				winner, e := ClaimHandoverFrom(ctx, s, "c", "B", "old-operation", v.Owner, v.Epoch)
				mustCoexist(t, e)
				mustCoexist(t, winner.Close())
				_, e = RecoverDeadOwner(ctx, s, "c", v, "C", func(context.Context) error { return nil })
				refused(t, e)
				after, e := ReadOwnership(ctx, s, "c")
				mustCoexist(t, e)
				if after.Owner != "B" {
					t.Fatal(after)
				}
				return
			}
			recovered, err := RecoverDeadOwner(ctx, s, "c", v, "C", func(context.Context) error {
				_, e := ClaimHandoverFrom(ctx, s, "c", "B", "old-operation", v.Owner, v.Epoch)
				refused(t, e)
				return nil
			})
			mustCoexist(t, err)
			task, err := s.GetTask(ctx, "c", "t")
			mustCoexist(t, err)
			if task.Status != "interrupted" || !task.WakeClaimed {
				t.Fatal(task)
			}
			mustCoexist(t, s.Append(WithOwnership(ctx, recovered), Turn{ConversationID: "c", Role: "assistant", Content: "new"}))
			mustCoexist(t, recovered.Close())
			_, err = ClaimHandoverFrom(ctx, s, "c", "B", "old-operation", v.Owner, v.Epoch)
			refused(t, err)
			_, err = RecoverDeadOwner(ctx, s, "c", v, "C", func(context.Context) error { return nil })
			refused(t, err)
			epoch, err := HandoverResult(ctx, s, "c", "C", RecoveryID("c", v, "C"))
			mustCoexist(t, err)
			after, err := ReadOwnership(ctx, s, "c")
			mustCoexist(t, err)
			if epoch != after.Epoch || after.Owner != "C" {
				t.Fatal(after)
			}
		})
	}
}

// A failed recovery must not consume the reservation or partially interrupt tasks.
func TestReservedRecoveryRollbackAndStaleConsent(t *testing.T) {
	ctx, _, s := ownershipFixture(t)
	o := take(t, ctx, s, "c", "A")
	c := WithOwnership(ctx, o)
	mustCoexist(t, s.EnsureConversation(c, "c", "", ""))
	mustCoexist(t, s.InsertTask(c, TaskRecord{ID: "t", ParentID: "c", Status: "accepted"}))
	mustCoexist(t, o.ReserveHandover(ctx, "B", "pending"))
	v, err := ReadOwnership(ctx, s, "c")
	mustCoexist(t, err)
	stale := v
	stale.Handover = "different"
	_, err = RecoverDeadOwner(ctx, s, "c", stale, "C", func(context.Context) error { return nil })
	refused(t, err)
	_, err = s.db.Exec(`CREATE TRIGGER reject_recovery BEFORE UPDATE ON conversation_ownership WHEN NEW.owner='C' BEGIN SELECT RAISE(ABORT,'injected'); END`)
	mustCoexist(t, err)
	_, err = RecoverDeadOwner(ctx, s, "c", v, "C", func(context.Context) error { return nil })
	refused(t, err)
	after, err := ReadOwnership(ctx, s, "c")
	mustCoexist(t, err)
	task, err := s.GetTask(ctx, "c", "t")
	mustCoexist(t, err)
	if after != v || task.Status != "accepted" {
		t.Fatalf("partial recovery: %+v %+v", after, task)
	}
	winner, err := ClaimHandoverFrom(ctx, s, "c", "B", "pending", v.Owner, v.Epoch)
	mustCoexist(t, err)
	defer winner.Close()
}
