package taskrun

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/recap"
)

type failingRecapStore struct {
	recap.Store
	failure error
}

func (s failingRecapStore) UpdateRecap(context.Context, string, string) error { return s.failure }

// A successful mock cloud executor still crosses the owned recap completion
// boundary before taskrun decides its terminal state. No models or credentials.
func TestCloudResultWithOwnedRecapOutcome(t *testing.T) {
	for _, mode := range []string{"inference", "write", "uncertain_callback", "executor"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
			t.Setenv("HOME", t.TempDir())
			failure := errors.New("mock failure")
			var st conversation.Store
			warnings := make(chan error, 1)
			m, store := newManager(t, func(ctx context.Context, spec dispatch.Spec) (dispatch.Result, error) {
				result := dispatch.Result{Text: "successful mock cloud response", IsCloud: true, Provider: "mock-cloud", Model: "mock-cloud-model", SubConversationID: spec.SubConversationID}
				if mode == "executor" {
					return result, failure
				}
				id := spec.SubConversationID
				if err := st.EnsureConversation(ctx, id, "", ""); err != nil {
					return result, err
				}
				if err := st.Append(ctx, conversation.Turn{ID: "recap-fixture", ConversationID: id, Role: "assistant", Content: result.Text, CreatedAt: time.Now()}); err != nil {
					return result, err
				}
				a := broker.NewAdmission("mock-host")
				owner := broker.Owner{Conversation: id, Host: "mock-host", Epoch: "mock-epoch"}
				if err := a.RegisterOwner(owner); err != nil {
					return result, err
				}
				var rs recap.Store = st
				if mode == "write" {
					rs = failingRecapStore{Store: st, failure: failure}
				}
				var observed error
				owned := recap.WithPersistenceOutcome(ctx, func() error { return observed })
				g := recap.New(rs, func(context.Context, string) (string, error) {
					if mode == "inference" {
						return "", failure
					}
					// Models a usage/persistence callback with an uncertain commit outcome.
					if mode == "uncertain_callback" {
						observed = failure
						return "", failure
					}
					return "fixture recap", nil
				}, 0, 10)
				defer g.Close(context.Background())
				work, err := g.ScheduleOwned(owned, owner, func(c context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
					ticket, err := a.AdmitWork(o, k)
					return c, ticket, err
				})
				if err != nil {
					return result, err
				}
				<-work.Done
				warnings <- work.Warning
				_, closeErr := a.CloseOwner(owner, nil)
				if mode == "inference" {
					if closeErr != nil {
						return result, closeErr
					}
				} else if !errors.Is(closeErr, broker.ErrAdmissionBusy) {
					return result, errors.New("write failure lost accounting fence")
				}
				return result, work.Err
			})
			st = store
			task, err := m.Launch(context.Background(), taskSpec())
			if err != nil {
				t.Fatal(err)
			}
			final, err := m.Wait(deadline(t), "parent", task.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := "failed"
			if mode == "inference" {
				want = "completed"
				if !errors.Is(<-warnings, failure) {
					t.Fatal("missing auxiliary warning")
				}
			}
			if final.Status != want {
				t.Fatalf("status=%s want=%s error=%s", final.Status, want, final.Error)
			}
			var result dispatch.Result
			if err := json.Unmarshal(final.ResultJSON, &result); err != nil {
				t.Fatal(err)
			}
			if result.Text != "successful mock cloud response" {
				t.Fatalf("lost cloud result: %+v", result)
			}
		})
	}
}
