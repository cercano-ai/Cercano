package taskrun

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
)

type lifecycleGate struct{ entered, release chan struct{} }

func newLifecycleGate() *lifecycleGate {
	return &lifecycleGate{make(chan struct{}), make(chan struct{})}
}
func (g *lifecycleGate) block() { close(g.entered); <-g.release }

type ownedBarrierStore struct {
	conversation.TaskStore
	insert, finish *lifecycleGate
	reject         bool
	fail           bool
}

func (s *ownedBarrierStore) InsertTask(ctx context.Context, r conversation.TaskRecord) error {
	s.insert.block()
	return s.TaskStore.InsertTask(ctx, r)
}
func (s *ownedBarrierStore) FinishTask(ctx context.Context, r conversation.TaskRecord) (bool, error) {
	s.finish.block()
	if s.fail {
		return false, errors.New("synthetic persistence failure")
	}
	if s.reject {
		return false, nil
	}
	return s.TaskStore.FinishTask(ctx, r)
}

func TestOwnedLifecycleAdmission(t *testing.T) {
	for _, outcome := range []string{"success", "stale", "rejected", "failed"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "owned.db")
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(conversation.PrepareCoexistence(ctx, path))
			store, err := conversation.OpenCoexistence(ctx, path, conversation.CurrentCoexistenceDeclaration())
			must(err)
			defer store.Close()
			owner, err := conversation.AcquireOwnership(ctx, store, "parent", "A")
			must(err)
			defer owner.Close()
			must(store.EnsureConversation(conversation.WithOwnership(ctx, owner), "parent", "", ""))
			admission := broker.NewAdmission("A")
			identity := broker.Owner{Conversation: "parent", Host: "A", Epoch: owner.Epoch()}
			must(admission.RegisterOwner(identity))
			insert, execute, finish, notify := newLifecycleGate(), newLifecycleGate(), newLifecycleGate(), newLifecycleGate()
			wrapped := &ownedBarrierStore{TaskStore: store.(conversation.TaskStore), insert: insert, finish: finish, reject: outcome == "rejected", fail: outcome == "failed"}
			m, err := NewPreparedOwned(wrapped, func(context.Context, dispatch.Spec) (dispatch.Result, error) {
				execute.block()
				return dispatch.Result{}, nil
			}, func(string) { notify.block() }, admission, func(context.Context, string) (*conversation.Ownership, error) { return owner, nil })
			must(err)
			busy := func() {
				t.Helper()
				if _, err := admission.CloseOwner(identity, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("expected busy: %v", err)
				}
			}
			launchCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			launched := make(chan conversation.TaskRecord, 1)
			launchErr := make(chan error, 1)
			go func() {
				r, e := m.Launch(launchCtx, dispatch.Spec{ConversationID: "parent", Task: "synthetic"})
				launched <- r
				launchErr <- e
			}()
			<-insert.entered
			busy()
			close(insert.release)
			task := <-launched
			must(<-launchErr)
			<-execute.entered
			cancel()
			busy()
			// Starting a second prepared manager cannot reconcile A's running task.
			b, err := NewPreparedOwned(store.(conversation.TaskStore), func(context.Context, dispatch.Spec) (dispatch.Result, error) { return dispatch.Result{}, nil }, nil, broker.NewAdmission("B"), func(context.Context, string) (*conversation.Ownership, error) { return nil, errors.New("not owned") })
			must(err)
			before, err := store.(conversation.TaskStore).GetTask(ctx, "parent", task.ID)
			must(err)
			if before.Status != "running" {
				t.Fatalf("B changed A task: %s", before.Status)
			}
			b.cancel()
			close(execute.release)
			<-finish.entered
			busy()
			if outcome == "stale" {
				// Deliberately violate host ordering to exercise the durable stale fence.
				must(owner.Release(ctx))
				replacement, e := conversation.AcquireOwnership(ctx, store, "parent", "B")
				must(e)
				defer replacement.Close()
			}
			close(finish.release)
			if outcome == "success" {
				<-notify.entered
				busy()
				close(notify.release)
				m.wg.Wait()
				_, err := admission.CloseOwner(identity, nil)
				must(err)
				got, err := store.(conversation.TaskStore).GetTask(ctx, "parent", task.ID)
				must(err)
				if got.Status != "completed" {
					t.Fatalf("status: %s", got.Status)
				}
			} else {
				m.wg.Wait()
				busy()
				select {
				case <-notify.entered:
					t.Fatal("notified unresolved outcome")
				default:
				}
				got, err := store.(conversation.TaskStore).GetTask(ctx, "parent", task.ID)
				must(err)
				if got.Terminal() {
					t.Fatalf("stale/rejected write applied: %s", got.Status)
				}
				if _, err := m.Wait(ctx, "parent", task.ID); err == nil {
					t.Fatal("missing persistence failure")
				}
			}
			m.cancel()
		})
	}
}
