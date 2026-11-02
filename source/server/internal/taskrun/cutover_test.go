package taskrun

import (
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/dispatch"
	"context"
	"testing"
)

func TestCutoverCancelsDescendantsButNotOtherConversations(t *testing.T) {
	started := make(chan string, 4)
	m, _ := newManager(t, func(ctx context.Context, s dispatch.Spec) (dispatch.Result, error) {
		started <- s.ConversationID
		<-ctx.Done()
		return dispatch.Result{Text: "partial"}, ctx.Err()
	})
	parent, err := m.Launch(context.Background(), taskSpec())
	if err != nil {
		t.Fatal(err)
	}
	childSpec := taskSpec()
	childSpec.ConversationID = parent.ChildID
	child, err := m.Launch(context.Background(), childSpec)
	if err != nil {
		t.Fatal(err)
	}
	otherSpec := taskSpec()
	otherSpec.ConversationID = "unrelated"
	other, err := m.Launch(context.Background(), otherSpec)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-deadline(t).Done():
			t.Fatal("tasks did not start")
		}
	}
	m.CancelConversation("parent", "")
	for _, pair := range [][2]string{{"parent", parent.ID}, {parent.ChildID, child.ID}} {
		for {
			record, err := m.Wait(deadline(t), pair[0], pair[1])
			if err != nil {
				t.Fatal(err)
			}
			if record.Terminal() {
				if record.Status != "stopped" {
					t.Fatalf("not stopped: %+v", record)
				}
				break
			}
		}
	}
	record, err := m.Inspect(context.Background(), "unrelated", other.ID)
	if err != nil || record.Status != "running" {
		t.Fatalf("unrelated work cancelled: %+v %v", record, err)
	}
	for _, p := range []string{"parent", parent.ChildID, child.ChildID} {
		spec := taskSpec()
		spec.ConversationID = p
		if _, err := m.Launch(context.Background(), spec); err == nil {
			t.Fatal("cutover admitted new descendant")
		}
	}
}

func TestCutoverDoesNotRevisitAnotherConversationsPriorEpoch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &Manager{cutoverParents: map[string]bool{"prior": true}, cutoverEpoch: map[string]string{"prior": "old"}, childParents: map[string]string{"fresh-child": "prior"}, active: map[string]*execution{"new-task": {parent: "fresh-child", owner: broker.Owner{Conversation: "fresh-child", Epoch: "new"}, cancel: cancel}}}
	m.CancelConversation("unrelated", "other-epoch")
	if ctx.Err() != nil {
		t.Fatal("a previous cutover cancelled a new epoch in another conversation")
	}
}
