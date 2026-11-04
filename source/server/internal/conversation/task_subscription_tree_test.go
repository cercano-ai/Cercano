package conversation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestTaskSubscriptionTreeResumeAndOwnership(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "tree.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tasks := s.(TaskStore)
	if err := tasks.InitTasks(ctx); err != nil {
		t.Fatal(err)
	}
	// Deliberately reverse lexical IDs: descendants must follow their parent,
	// even across snapshot pages, so restored CLI titles have the right ancestry.
	records := []TaskRecord{{ID: "z", ParentID: "root", ChildID: "parent", Status: "accepted", Task: "parent"}}
	for i := 0; i < TaskSubscriptionPageSize+2; i++ {
		records = append(records, TaskRecord{ID: fmt.Sprintf("a%03d", i), ParentID: "parent", ChildID: fmt.Sprintf("leaf%03d", i), Status: "accepted", Task: "leaf"})
	}
	records = append(records, TaskRecord{ID: "unrelated", ParentID: "elsewhere", ChildID: "private", Status: "accepted", Task: "private"})
	for _, r := range records {
		if err := tasks.InsertTask(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	sub := s.(TaskSubscriptionStore)
	page, err := sub.TaskSnapshotPage(ctx, "root", "")
	if err != nil || len(page) != TaskSubscriptionPageSize || page[0].ID != "z" {
		t.Fatalf("first page=%v err=%v", page, err)
	}
	next, err := sub.TaskSnapshotPage(ctx, "root", page[len(page)-1].ID)
	if err != nil || len(next) != 3 {
		t.Fatalf("resume=%v err=%v", next, err)
	}
	if _, err := tasks.GetTask(ctx, "root", "a000"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("ancestor control: %v", err)
	}
	if _, err := tasks.GetTask(ctx, "parent", "a000"); err != nil {
		t.Fatal(err)
	}
	// Resuming the durable tree preserves immediate-parent metadata.
	if next[0].ParentID != "parent" {
		t.Fatalf("lost ancestry: %+v", next[0])
	}
}
