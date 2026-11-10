package conversation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

func TestDeadOwnerUncertainCommitReconciliation(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			ctx, _, s := ownershipFixture(t)
			root := take(t, ctx, s, "root", "A")
			child := take(t, ctx, s, "child", "A")
			c := WithOwnership(ctx, root, child)
			mustCoexist(t, s.EnsureConversation(c, "root", "", ""))
			mustCoexist(t, s.EnsureSubagentConversation(c, "child", "root", "", "", nil))
			before, err := ReadOwnership(ctx, s, "root")
			mustCoexist(t, err)
			mustCoexist(t, root.Close())
			mustCoexist(t, child.Close())
			dead := func(context.Context) error { return nil }
			injected := errors.New("lost commit acknowledgement")
			uncertain, err := recoverDeadOwner(ctx, s, "root", before, "B", dead, func(tx *sql.Tx) error {
				if committed {
					mustCoexist(t, tx.Commit())
				} else {
					mustCoexist(t, tx.Rollback())
				}
				return injected
			})
			if !errors.Is(err, injected) || uncertain == nil || uncertain.usable.Load() {
				t.Fatalf("must not publish usable/ready capability: %v %v", uncertain, err)
			}
			defer uncertain.Close()
			for _, id := range []string{"root", "child"} {
				_, e := AcquireOwnership(ctx, s, id, "C")
				refused(t, e)
			}
			refused(t, s.Append(WithOwnership(ctx, uncertain), Turn{ConversationID: "root", Role: "user", Content: "must not write"}))
			_, err = RecoverDeadOwner(ctx, s, "root", before, "B", dead)
			refused(t, err)
			// Explicitly inspect durable state before releasing retained exclusion.
			after, err := ReadOwnership(ctx, s, "root")
			mustCoexist(t, err)
			if committed {
				if after.Owner != "B" || after.Epoch != uncertain.Epoch() {
					t.Fatal(after)
				}
			} else if after != before {
				t.Fatal(after)
			}
			mustCoexist(t, uncertain.Close())
			retry, err := RecoverDeadOwner(ctx, s, "root", before, "B", dead)
			if committed {
				refused(t, err)
			} else {
				mustCoexist(t, err)
				defer retry.Close()
				after, err = ReadOwnership(ctx, s, "root")
				mustCoexist(t, err)
			}
			receipt, err := HandoverResult(ctx, s, "root", "B", RecoveryID("root", before, "B"))
			mustCoexist(t, err)
			if receipt != after.Epoch {
				t.Fatal("retry duplicated epoch")
			}
			var count int
			mustCoexist(t, s.db.QueryRow(`SELECT count(*) FROM conversation_handovers WHERE conversation_id='root'`).Scan(&count))
			if count != 1 {
				t.Fatalf("receipts=%d", count)
			}
		})
	}
}

func TestDeadOwnerNotificationDoesNotReplayAction(t *testing.T) {
	ctx, _, s := ownershipFixture(t)
	o := take(t, ctx, s, "c", "A")
	c := WithOwnership(ctx, o)
	mustCoexist(t, s.EnsureConversation(c, "c", "", ""))
	for _, id := range []string{"running", "completed", "claimed"} {
		mustCoexist(t, s.InsertTask(c, TaskRecord{ID: id, ParentID: "c", Status: "accepted"}))
		mustCoexist(t, s.StartTask(c, "c", id))
		if id != "running" {
			won, err := s.FinishTask(c, TaskRecord{ID: id, ParentID: "c", Status: "completed", ResultJSON: []byte(`{"result":"durable"}`)})
			mustCoexist(t, err)
			if !won {
				t.Fatal("finish")
			}
		}
	}
	won, err := s.ClaimTaskWake(c, "c", "claimed")
	mustCoexist(t, err)
	if !won {
		t.Fatal("claim")
	}
	before, err := ReadOwnership(ctx, s, "c")
	mustCoexist(t, err)
	mustCoexist(t, o.Close())
	recovered, err := RecoverDeadOwner(ctx, s, "c", before, "B", func(context.Context) error { return nil })
	mustCoexist(t, err)
	defer recovered.Close()
	c = WithOwnership(ctx, recovered)
	for _, id := range []string{"running", "completed", "claimed"} {
		task, err := s.GetTask(ctx, "c", id)
		mustCoexist(t, err)
		if !task.WakeClaimed || !task.NotificationPending {
			t.Fatal(task)
		}
		if id == "running" {
			if task.Status != "interrupted" {
				t.Fatal(task)
			}
		} else if task.Status != "completed" || string(task.ResultJSON) != `{"result":"durable"}` {
			t.Fatal(task)
		}
		refused(t, s.StartTask(c, "c", id))
		won, err := s.ClaimTaskWake(c, "c", id)
		mustCoexist(t, err)
		if won {
			t.Fatal("automatic wake replay")
		}
		won, err = s.FinishTask(c, TaskRecord{ID: id, ParentID: "c", Status: "completed", ResultJSON: []byte(`"overwrite"`)})
		mustCoexist(t, err)
		if won {
			t.Fatal("terminal result overwritten")
		}
	}
	pending, err := s.PendingTasks(ctx, "c")
	mustCoexist(t, err)
	if len(pending) != 3 {
		t.Fatal(pending)
	}
	// Notification acknowledgement consumes evidence, not execution authority.
	mustCoexist(t, s.HandleTaskNotification(c, "c", "completed"))
	task, err := s.GetTask(ctx, "c", "completed")
	mustCoexist(t, err)
	if task.NotificationPending || !task.WakeClaimed || string(task.ResultJSON) != `{"result":"durable"}` {
		t.Fatal(task)
	}
}
