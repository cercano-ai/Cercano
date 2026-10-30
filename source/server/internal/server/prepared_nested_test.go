package server

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/llm"
)

func TestPreparedNestedDispatchThreeLevels(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	child, finishChild, err := s.beginPreparedDispatch(ctx, owner.Conversation, "nested-child")
	if err != nil {
		t.Fatal(err)
	}
	st := s.coexistence.store
	if err := st.EnsureSubagentConversation(child, "nested-child", owner.Conversation, "", "fixture", nil); err != nil {
		t.Fatal(err)
	}
	grandchild, finishGrandchild, err := s.beginPreparedDispatch(child, "nested-child", "nested-grandchild")
	if err != nil {
		t.Fatalf("grandchild acquisition: %v", err)
	}
	if err := st.EnsureSubagentConversation(grandchild, "nested-grandchild", "nested-child", "", "fixture", nil); err != nil {
		t.Fatal(err)
	}
	s.persistSvc.PersistTurn(grandchild, "nested-grandchild", llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockText, Text: "grandchild"}}})
	if err := persistsvc.WriteFailure(grandchild); err != nil {
		t.Fatal(err)
	}
	events := st.(conversation.DispatchEventStore)
	ev := conversation.DispatchEvent{ConversationID: "nested-grandchild", Seq: 1, Kind: "completed", PayloadJSON: "{}"}
	if err := events.AppendDispatchEvent(child, ev); err == nil {
		t.Fatal("child context authorized grandchild evidence")
	}
	if err := events.AppendDispatchEvent(grandchild, ev); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.beginPreparedDispatch(child, owner.Conversation, "wrong-parent"); err == nil {
		t.Fatal("ancestor authority accepted")
	}
	if _, _, err := s.beginPreparedDispatch(child, "sibling", "wrong-sibling"); err == nil {
		t.Fatal("sibling authority accepted")
	}
	forged := WithPreparedTaskOwnership(child, owner, s.coexistence.owners[owner.Conversation])
	if _, _, err := s.beginPreparedDispatch(forged, owner.Conversation, "forged-parent"); err == nil {
		t.Fatal("inherited scope accepted a different exact owner")
	}
	if err := finishGrandchild(nil); err != nil {
		t.Fatal(err)
	}
	if err := finishChild(nil); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"nested-child", "nested-grandchild"} {
		state, err := conversation.ReadOwnership(ctx, s.coexistence.store, id)
		if err != nil || state.State != "released" {
			t.Fatalf("%s: %+v %v", id, state, err)
		}
	}
	turns, err := s.coexistence.store.GetTurns(ctx, "nested-grandchild")
	if err != nil || len(turns) != 1 {
		t.Fatalf("grandchild transcript: %v %v", turns, err)
	}
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
}

func waitPreparedScopeClosed(t *testing.T, ctx context.Context) {
	t.Helper()
	scope := ctx.Value(preparedDispatchKey{}).(*preparedDispatchScope)
	deadline := time.Now().Add(5 * time.Second)
	for {
		scope.mu.Lock()
		closed := scope.closed
		scope.mu.Unlock()
		if closed {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("scope did not fence admission")
		}
		runtime.Gosched()
	}
}

func TestPreparedNestedDispatchBottomUpJoin(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "write-failure"}[fail], func(t *testing.T) {
			s, ctx, owner, _ := preparedWakeFixture(t)
			child, finishChild, err := s.beginPreparedDispatch(ctx, owner.Conversation, "join-child")
			if err != nil {
				t.Fatal(err)
			}
			grandchild, finishGrandchild, err := s.beginPreparedDispatch(child, "join-child", "join-grandchild")
			if err != nil {
				t.Fatal(err)
			}
			// Model a producer's final writeback registered in the grandchild scope.
			producerDone := make(chan struct{})
			scope := grandchild.Value(preparedDispatchKey{}).(*preparedDispatchScope)
			scope.waits = append(scope.waits, func() error { <-producerDone; return nil })
			childDone, grandchildDone := make(chan error, 1), make(chan error, 1)
			go func() { childDone <- finishChild(nil) }()
			waitPreparedScopeClosed(t, child)
			// A waiting branch must not fence an independently admitted sibling.
			_, finishSibling, err := s.beginPreparedDispatch(ctx, owner.Conversation, "join-sibling")
			if err != nil {
				t.Fatal(err)
			}
			if err := finishSibling(nil); err != nil {
				t.Fatalf("sibling blocked by descendant join: %v", err)
			}
			if _, _, err := s.beginPreparedDispatch(child, "join-child", "late-child"); !errors.Is(err, broker.ErrAdmissionClosed) {
				t.Fatalf("late admission: %v", err)
			}
			select {
			case err := <-childDone:
				t.Fatalf("parent escaped descendant: %v", err)
			default:
			}
			// This takes the lifecycle mutex: a blocked join must not hold it.
			if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("root escaped: %v", err)
			}
			if fail {
				persistsvc.ReportWriteFailure(grandchild, errors.New("injected grandchild write"))
			}
			go func() { grandchildDone <- finishGrandchild(nil) }()
			waitPreparedScopeClosed(t, grandchild)
			select {
			case err := <-grandchildDone:
				t.Fatalf("grandchild escaped producer: %v", err)
			default:
			}
			close(producerDone)
			for _, done := range []chan error{grandchildDone, childDone} {
				select {
				case err := <-done:
					if (err != nil) != fail {
						t.Fatalf("completion: %v fail=%v", err, fail)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("bottom-up join stuck")
				}
			}
			err = s.ReleaseRetained(ctx, owner, nil)
			if fail {
				if !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("lost failure blocker: %v", err)
				}
				for _, id := range []string{"join-child", "join-grandchild"} {
					_ = s.coexistence.owners[id].Close()
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreparedNestedDispatchReleaseFailure(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	child, finishChild, err := s.beginPreparedDispatch(ctx, owner.Conversation, "release-child")
	if err != nil {
		t.Fatal(err)
	}
	_, finishGrandchild, err := s.beginPreparedDispatch(child, "release-child", "release-grandchild")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", s.coexistence.options.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_nested_release BEFORE UPDATE ON conversation_ownership WHEN NEW.conversation_id='release-grandchild' AND NEW.state='released' BEGIN SELECT RAISE(FAIL, 'injected nested release failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := finishGrandchild(nil); err == nil {
		t.Fatal("release failure lost")
	}
	if err := finishChild(nil); err == nil {
		t.Fatal("descendant failure lost")
	}
	if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("root blocker lost: %v", err)
	}
	for _, id := range []string{"release-child", "release-grandchild"} {
		state, err := conversation.ReadOwnership(ctx, s.coexistence.store, id)
		if err != nil || state.State != "owned" {
			t.Fatalf("failed release %s: %+v %v", id, state, err)
		}
		if s.coexistence.owners[id] == nil {
			t.Fatal("failed release capability lost")
		}
		_ = s.coexistence.owners[id].Close()
	}
}

func TestPreparedNestedDispatchRegistrationAgainstClose(t *testing.T) {
	for i := 0; i < 20; i++ {
		s, ctx, owner, _ := preparedWakeFixture(t)
		child, finishChild, err := s.beginPreparedDispatch(ctx, owner.Conversation, "racing-child")
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		finished := make(chan error, 2)
		go func() { <-start; finished <- finishChild(nil) }()
		go func() {
			<-start
			_, complete, err := s.beginPreparedDispatch(child, "racing-child", "racing-grandchild")
			if err == nil {
				err = complete(nil)
			} else if errors.Is(err, broker.ErrAdmissionClosed) || errors.Is(err, broker.ErrAdmissionStale) {
				err = nil
			}
			finished <- err
		}()
		close(start)
		for j := 0; j < 2; j++ {
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("registration/close deadlock")
			}
		}
		if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
			t.Fatal(err)
		}
	}
}
