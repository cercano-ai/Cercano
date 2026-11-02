package server

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
)

// Disk SQLite + real gRPC/SDK, with fresh services and client connections on
// every restart. No process/session visibility state can mask a missing write.
func TestExplicitChildReopenPersistsVisibility(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	path := filepath.Join(t.TempDir(), "conversations.db")
	s, store := newServerWithStore(t, path)
	for _, id := range []string{"active", "finished", "closed"} {
		seedObservedTask(t, s, "owner", "task-"+id, id)
		if err := store.Append(t.Context(), conversation.Turn{ID: "turn-" + id, ConversationID: id, Role: "assistant", Content: "retained transcript " + id, CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	tasks := store.(conversation.TaskStore)
	if _, err := tasks.FinishTask(t.Context(), conversation.TaskRecord{ID: "task-finished", ParentID: "owner", ChildID: "finished", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	before := map[string]*conversation.TaskRecord{}
	for _, id := range []string{"active", "finished", "closed"} {
		task, err := tasks.GetTask(t.Context(), "owner", "task-"+id)
		if err != nil {
			t.Fatal(err)
		}
		before[id] = &task
	}
	client := taskObserverClient(t, s)
	assertVisible := func(client *agentclient.Client, want ...string) {
		t.Helper()
		children, err := client.ListSubAgents(t.Context(), "owner")
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, child := range children {
			got[child.ID] = true
		}
		expected := map[string]bool{}
		for _, id := range want {
			expected[id] = true
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("visible=%v want=%v", got, expected)
		}
	}
	restart := func() {
		t.Helper()
		client.Close()
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		s, store = newServerWithStore(t, path)
		tasks = store.(conversation.TaskStore)
		client = taskObserverClient(t, s)
	}
	assertVisible(client, "active", "finished", "closed")
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := client.DismissSubAgent(canceled, "active"); err == nil {
		t.Fatal("canceled dismissal succeeded")
	}
	assertVisible(client, "active", "finished", "closed")
	if err := client.DismissSubAgent(t.Context(), "closed"); err != nil {
		t.Fatal(err)
	}
	// Hydration (also used by automatic child restoration) is NOT explicit reopen.
	if _, err := client.ResumeConversation(t.Context(), "closed"); err != nil {
		t.Fatal(err)
	}
	assertVisible(client, "active", "finished")
	restart()
	assertVisible(client, "active", "finished")
	if err := client.ReopenSubAgent(canceled, "closed"); err == nil {
		t.Fatal("canceled reopen succeeded")
	}
	assertVisible(client, "active", "finished")
	if err := client.ReopenSubAgent(t.Context(), "closed"); err != nil {
		t.Fatal(err)
	}
	assertVisible(client, "active", "finished", "closed")
	restart()
	assertVisible(client, "active", "finished", "closed")
	if err := client.DismissSubAgent(t.Context(), "closed"); err != nil {
		t.Fatal(err)
	}
	restart()
	assertVisible(client, "active", "finished")
	for _, id := range []string{"active", "finished", "closed"} {
		task, err := tasks.GetTask(t.Context(), "owner", "task-"+id)
		if err != nil || !reflect.DeepEqual(&task, before[id]) {
			t.Fatalf("visibility changed task %s: %v %v", id, task, err)
		}
		turns, err := client.ResumeConversation(t.Context(), id)
		if err != nil || len(turns) != 1 || turns[0].Content != "retained transcript "+id {
			t.Fatalf("lost transcript: %s %v %v", id, turns, err)
		}
	}
}
