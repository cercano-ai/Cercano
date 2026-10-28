package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/recap"
)

// Model a store adapter whose persistence callback has no error return. The
// callback reports a real fenced SQLite rejection through its supplied context.
type recapBoundaryStore struct {
	recap.Store
	boundary string
	entered  chan struct{}
	resume   chan struct{}
	failure  error
}

func (s *recapBoundaryStore) callback(ctx context.Context, id string) {
	close(s.entered)
	<-s.resume // Deliberately ignore cancellation: Close must still wait.
	s.failure = s.Store.UpdateRecap(context.Background(), id, "must not persist")
	ReportWriteFailure(ctx, s.failure)
}
func (s *recapBoundaryStore) UpdateRecap(ctx context.Context, id, line string) error {
	if s.boundary == "recap_callback" {
		s.callback(ctx, id)
		return nil
	}
	return s.Store.UpdateRecap(ctx, id, line)
}
func (s *recapBoundaryStore) SetGeneratedTitle(ctx context.Context, id, title string) error {
	if s.boundary == "recap_callback" {
		return s.Store.SetGeneratedTitle(context.WithoutCancel(ctx), id, title)
	}
	if s.boundary == "title_callback" {
		s.callback(ctx, id)
		return nil
	}
	close(s.entered)
	<-s.resume
	// Ignore cancellation only to model an already executing final durable save.
	return s.Store.SetGeneratedTitle(context.WithoutCancel(ctx), id, title)
}

func TestOwnedRecapFinalBoundary(t *testing.T) {
	for _, boundary := range []string{"title_save", "recap_callback", "title_callback"} {
		t.Run(boundary, func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "history.db")
			if err := conversation.PrepareCoexistence(ctx, path); err != nil {
				t.Fatal(err)
			}
			store, err := conversation.OpenCoexistence(ctx, path, conversation.CurrentCoexistenceDeclaration())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			cap, err := conversation.AcquireOwnership(ctx, store, "c", "host")
			if err != nil {
				t.Fatal(err)
			}
			defer cap.Close()
			owned := conversation.WithOwnership(ctx, cap)
			if err := store.EnsureConversation(owned, "c", "", ""); err != nil {
				t.Fatal(err)
			}
			if err := store.Append(owned, conversation.Turn{ID: "t", ConversationID: "c", Role: "user", Content: "hello", CreatedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			owner := broker.Owner{Conversation: "c", Host: "host", Epoch: cap.Epoch()}
			admission := broker.NewAdmission("host")
			if err := admission.RegisterOwner(owner); err != nil {
				t.Fatal(err)
			}
			probe := &recapBoundaryStore{Store: store, boundary: boundary, entered: make(chan struct{}), resume: make(chan struct{})}
			calls, admissions := 0, 0
			g := recap.New(probe, func(context.Context, string) (string, error) { calls++; return "Completed Work", nil }, 0, 10)
			released := false
			defer func() {
				if !released {
					close(probe.resume)
				}
				_ = g.Close(context.Background())
			}()
			var observed []error
			parent := WithWriteFailure(ctx, func(err error) { observed = append(observed, err) })
			work, err := ScheduleOwnedRecap(parent, g, owner, func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
				admissions++
				if o != owner {
					return nil, nil, broker.ErrAdmissionStale
				}
				ticket, err := admission.AdmitWork(o, k)
				return conversation.WithOwnership(ctx, cap), ticket, err
			})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-probe.entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if _, err := admission.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("final boundary owner close: %v", err)
			}
			budget, stop := context.WithTimeout(ctx, 20*time.Millisecond)
			err = g.Close(budget)
			stop()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Close returned before final callback: %v", err)
			}
			select {
			case <-work.Done:
				t.Fatal("work completed inside callback")
			default:
			}
			close(probe.resume)
			released = true
			select {
			case <-work.Done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if err := g.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if admissions != 1 {
				t.Fatalf("admissions=%d", admissions)
			}
			info, err := store.Get(owned, "c")
			if err != nil {
				t.Fatal(err)
			}
			if boundary == "title_save" {
				if work.Err != nil || calls != 2 || info.Recap != "Completed Work" || info.Title != "Completed Work" {
					t.Fatalf("final save: calls=%d info=%+v err=%v", calls, info, work.Err)
				}
				if err := work.Ticket.Complete(); !errors.Is(err, broker.ErrAdmissionStale) {
					t.Fatalf("ticket not completed exactly once: %v", err)
				}
				if _, err := admission.CloseOwner(owner, nil); err != nil {
					t.Fatal(err)
				}
			} else {
				if probe.failure == nil || len(observed) != 1 || !errors.Is(work.Err, probe.failure) {
					t.Fatalf("reported failure lost: write=%v observed=%v work=%v calls=%d info=%+v", probe.failure, observed, work.Err, calls, info)
				}
				if boundary == "recap_callback" && (calls != 1 || info.Recap != "" || info.Title != "hello") {
					t.Fatalf("dependent title published: calls=%d info=%+v", calls, info)
				}
				if boundary == "title_callback" && (calls != 2 || info.Title != "hello") {
					t.Fatalf("title published: calls=%d info=%+v", calls, info)
				}
				if _, err := admission.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("failure blocker lost: %v", err)
				}
				// Explicit reconciliation is required; completion cannot erase the blocker.
				if err := work.Ticket.Complete(); !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("failed ticket completed: %v", err)
				}
				if err := work.Ticket.ResolvePersistence(); err != nil {
					t.Fatal(err)
				}
				if err := work.Ticket.Complete(); err != nil {
					t.Fatal(err)
				}
				if err := work.Ticket.Complete(); !errors.Is(err, broker.ErrAdmissionStale) {
					t.Fatalf("duplicate completion: %v", err)
				}
				if _, err := admission.CloseOwner(owner, nil); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
