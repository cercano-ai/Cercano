package server

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/locus"
	"cercano/source/server/internal/taskrun"
	"google.golang.org/grpc/metadata"
)

// Dispatch persists the task before route selection. A routing failure therefore
// has a real child identity but no conversation row or transcript.
func TestPinnedFailedTaskTabUsesExistingDismissal(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	store := s.persistSvc.Store()
	tasks := store.(conversation.TaskStore)
	// Exercise the real failure path, rather than inventing a task record.
	s.closeTaskManager()
	engine := dispatch.NewEngine(func() inference.Tiers { return inference.Tiers{} }, func() locus.Mode { return locus.OpenOnly }, nil)
	manager, err := taskrun.NewPreparedOwnedCompletion(tasks, engine.Dispatch, nil, s.coexistence.admission, s.resolvePreparedTaskOwner)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())
	task, err := manager.Launch(ctx, dispatch.Spec{Mode: dispatch.Agentic, ConversationID: "parent", Task: "no route", Tools: []string{"Read"}})
	if err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	task, err = manager.Wait(wait, "parent", task.ID)
	if err != nil || task.Status != "failed" || task.Error == "" {
		t.Fatalf("expected routing failure: %+v %v", task, err)
	}
	before, err := tasks.GetTask(ctx, "parent", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, task.ChildID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected absent transcript: %v", err)
	}
	incoming := metadata.NewIncomingContext(context.Background(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "terminal"))
	_, hold, _, err := s.BeginTerminalStream(incoming, "parent", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Detach()
	client := taskObserverClient(t, s)
	bound := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "terminal"))
	if err := client.DismissSubAgent(bound, task.ChildID); err != nil {
		t.Fatal(err)
	}
	assertVisible := func(want int) {
		t.Helper()
		page, err := store.(conversation.TaskSubscriptionStore).TaskSnapshotPage(ctx, "parent", "")
		count := 0
		for _, row := range page {
			if row.ID == task.ID {
				count++
			}
		}
		if err != nil || count != want {
			t.Fatalf("visible=%v err=%v want=%d", page, err, want)
		}
	}
	assertVisible(0)
	if err := client.ReopenSubAgent(bound, task.ChildID); err != nil {
		t.Fatal(err)
	}
	assertVisible(1)
	if err := client.DismissSubAgent(bound, task.ChildID); err != nil {
		t.Fatal(err)
	}
	assertVisible(0)
	// A second store connection has no UI or server cache. Existing schema-2
	// files must reopen unchanged and still suppress this tab.
	reopened, err := conversation.OpenCoexistence(t.Context(), s.coexistence.options.StorePath, conversation.CurrentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	page, err := reopened.(conversation.TaskSubscriptionStore).TaskSnapshotPage(t.Context(), "parent", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range page {
		if row.ID == task.ID {
			t.Fatal("dismissal lost on store reopen")
		}
	}
	after, err := tasks.GetTask(ctx, "parent", task.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("task changed: %v %v", after, err)
	}
	info, err := store.Get(ctx, task.ChildID)
	if err != nil || info.Kind != "subagent" || info.ParentID != "parent" || info.TurnCount != 0 {
		t.Fatalf("invalid child metadata: %+v %v", info, err)
	}
	if err := client.DismissSubAgent(bound, "unrelated"); err == nil {
		t.Fatal("unrelated child admitted")
	}
}

func TestMissingTaskTabRepairRefusesUnsafeCases(t *testing.T) {
	for _, mode := range []string{"wrong-epoch", "wrong-terminal", "active", "already-owned"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, owner, tasks := preparedWakeFixture(t)
			task := conversation.TaskRecord{ID: "repair", ParentID: "parent", ChildID: "orphan", Status: "accepted", CreatedAt: time.Now(), UpdatedAt: time.Now()}
			if err := tasks.InsertTask(ctx, task); err != nil {
				t.Fatal(err)
			}
			if mode != "active" {
				task.Status = "failed"
				if _, err := tasks.FinishTask(ctx, task); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "already-owned" {
				_, finish, err := s.beginPreparedDispatch(ctx, "parent", "orphan")
				if err != nil {
					t.Fatal(err)
				}
				defer finish(nil)
			}
			epoch, terminal := owner.Epoch, "terminal"
			if mode == "wrong-epoch" {
				epoch = "stale"
			}
			incoming := metadata.NewIncomingContext(t.Context(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "terminal"))
			_, hold, _, err := s.BeginTerminalStream(incoming, "parent", 0)
			if err != nil {
				t.Fatal(err)
			}
			defer hold.Detach()
			if mode == "wrong-terminal" {
				terminal = "foreign"
			}
			bound := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", epoch, "cercano-terminal-attachment", terminal))
			if err := taskObserverClient(t, s).DismissSubAgent(bound, "orphan"); err == nil {
				t.Fatal("unsafe repair admitted")
			}
			if _, err := s.persistSvc.Store().Get(ctx, "orphan"); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("unauthorized row created: %v", err)
			}
		})
	}
}
