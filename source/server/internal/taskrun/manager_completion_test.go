package taskrun

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
)

func TestOwnedCompletionBridge(t *testing.T) {
	for _, mode := range []string{"success", "error", "stale"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(t.TempDir(), "owned.db")
			must(conversation.PrepareCoexistence(ctx, path))
			store, err := conversation.OpenCoexistence(ctx, path, conversation.CurrentCoexistenceDeclaration())
			must(err)
			defer store.Close()
			owner, err := conversation.AcquireOwnership(ctx, store, "parent", "A")
			must(err)
			defer owner.Close()
			must(store.EnsureConversation(conversation.WithOwnership(ctx, owner), "parent", "", ""))
			identity := broker.Owner{Conversation: "parent", Host: "A", Epoch: owner.Epoch()}
			admission := broker.NewAdmission("A")
			must(admission.RegisterOwner(identity))
			execute, callback := newLifecycleGate(), newLifecycleGate()
			var calls, resolutions atomic.Int32
			var queued *broker.WorkTicket
			callbackError := errors.New("queue unavailable")
			type valueKey struct{}
			var m *Manager
			m, err = NewPreparedOwnedCompletion(store.(conversation.TaskStore), func(context.Context, dispatch.Spec) (dispatch.Result, error) {
				execute.block()
				return dispatch.Result{}, nil
			}, func(c context.Context, o broker.Owner, parent string) error {
				calls.Add(1)
				// Reentrant lifecycle inspection must not deadlock on the manager mutex.
				m.UnresolvedTerminalOutcomes(parent)
				callback.block()
				if c.Err() != nil || c.Value(valueKey{}) != "launch" || o != identity || parent != "parent" {
					return errors.New("lost launch context or exact owner")
				}
				if mode != "stale" {
					if err := store.EnsureConversation(c, "parent", "", ""); err != nil {
						return err // proves the launch ownership capability survived cancellation
					}
				}
				if mode == "error" {
					return callbackError
				}
				if mode == "stale" {
					// Retained context must still carry the OLD capability, not the new
					// owner's capability or an unscoped background context.
					if err := store.EnsureConversation(c, "parent", "", ""); err == nil {
						return errors.New("stale capability accepted")
					}
				}
				var err error
				queued, err = admission.AdmitWork(o, broker.CallbackWork)
				return err
			}, admission, func(context.Context, string) (*conversation.Ownership, error) {
				resolutions.Add(1)
				// This concrete reentry deadlocked when Launch held m.mu around resolver.
				m.UnresolvedTerminalOutcomes("")
				return owner, nil
			})
			must(err)
			defer m.cancel()
			launchCtx, cancel := context.WithCancel(context.WithValue(ctx, valueKey{}, "launch"))
			defer cancel()
			type launched struct {
				task conversation.TaskRecord
				err  error
			}
			launch := make(chan launched, 1)
			go func() {
				task, e := m.Launch(launchCtx, dispatch.Spec{ConversationID: "parent", Task: "work"})
				launch <- launched{task, e}
			}()
			var task conversation.TaskRecord
			select {
			case r := <-launch:
				must(r.err)
				task = r.task
			case <-time.After(5 * time.Second):
				t.Fatal("ownership resolver reentry deadlocked")
			}
			<-execute.entered
			cancel()
			m.mu.Lock()
			wake := m.registerWaitWakeLocked("parent")
			m.mu.Unlock()
			close(execute.release)
			<-callback.entered
			if _, err := admission.CloseOwner(identity, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("callback not blocking closure: %v", err)
			}
			select {
			case <-wake:
				t.Fatal("completion signaled before callback success")
			default:
			}
			if mode == "stale" {
				must(owner.Release(ctx))
				replacement, e := conversation.AcquireOwnership(ctx, store, "parent", "B")
				must(e)
				defer replacement.Close()
				if replacement.Epoch() == identity.Epoch {
					t.Fatal("epoch did not change")
				}
			}
			close(callback.release)
			m.wg.Wait()
			if resolutions.Load() != 1 || calls.Load() != 1 {
				t.Fatal("ownership re-resolved or callback replayed")
			}
			if mode == "error" {
				if !errors.Is(m.UnresolvedTerminalOutcomes("parent")[task.ID], callbackError) {
					t.Fatal("missing explicit callback failure")
				}
				for i := 0; i < 2; i++ {
					if _, e := m.Wait(ctx, "parent", task.ID); !errors.Is(e, callbackError) {
						t.Fatalf("Wait: %v", e)
					}
				}
				select {
				case <-wake:
					t.Fatal("failed callback signaled completion")
				default:
				}
				if _, e := admission.CloseOwner(identity, nil); !errors.Is(e, broker.ErrAdmissionBusy) {
					t.Fatalf("failed callback released ticket: %v", e)
				}
				if calls.Load() != 1 {
					t.Fatal("callback replayed")
				}
			} else {
				if outcomes := m.UnresolvedTerminalOutcomes(""); len(outcomes) != 0 {
					t.Fatalf("callback failed: %v", outcomes)
				}
				if queued == nil {
					t.Fatal("no queued work")
				}
				if _, e := admission.CloseOwner(identity, nil); !errors.Is(e, broker.ErrAdmissionBusy) {
					t.Fatalf("queue handoff gap: %v", e)
				}
				must(queued.Complete())
				_, e := admission.CloseOwner(identity, nil)
				must(e)
				select {
				case <-wake:
				default:
					t.Fatal("missing success wake")
				}
			}
		})
	}
}
