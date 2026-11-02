package server

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/metadata"
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

func TestPinnedParentChildVisibilityAdmission(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	childCtx, finish, err := s.beginPreparedDispatch(ctx, "parent", "child")
	if err != nil {
		t.Fatal(err)
	}
	defer finish(nil)
	if err := s.persistSvc.Store().EnsureSubagentConversation(childCtx, "child", "parent", "", "", nil); err != nil {
		t.Fatal(err)
	}
	incoming := metadata.NewIncomingContext(t.Context(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "terminal"))
	_, hold, _, err := s.BeginTerminalStream(incoming, "parent", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Detach()
	client := taskObserverClient(t, s)
	bound := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "terminal"))
	if err := client.DismissSubAgent(bound, "child"); err != nil {
		t.Fatal(err)
	}
	if err := client.ReopenSubAgent(bound, "child"); err != nil {
		t.Fatal(err)
	}
	if err := client.DismissSubAgent(bound, "unrelated"); err == nil {
		t.Fatal("unrelated admitted")
	}
	wrongHost := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("cercano-host-instance", "foreign-host", "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "terminal"))
	if err := client.ReopenSubAgent(wrongHost, "child"); err == nil {
		t.Fatal("wrong host admitted")
	}
	wrongTerminal := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "unrelated-terminal"))
	if err := client.ReopenSubAgent(wrongTerminal, "child"); err == nil {
		t.Fatal("conflicting terminal admitted")
	}
	bad := owner
	bad.Epoch = "wrong"
	if _, err := s.DismissSubAgent(foregroundBinding(t.Context(), bad), &proto.DismissSubAgentRequest{ConversationId: "child"}); err == nil {
		t.Fatal("wrong epoch admitted")
	}
	replacement := metadata.NewIncomingContext(t.Context(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "replacement", "cercano-takeover-terminal", "terminal"))
	_, a, _, err := s.BeginTerminalStream(replacement, "parent", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Detach()
	if err := client.ReopenSubAgent(bound, "child"); err == nil {
		t.Fatal("revoked caller admitted")
	}
}
