package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/conversation"
)

func TestOwnedCompactionPersistenceBridge(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "durable", true: "failed_usage"}[fail], func(t *testing.T) {
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
			owner := broker.Owner{Conversation: "c", Host: "host", Epoch: cap.Epoch()}
			admission := broker.NewAdmission("host")
			if err := admission.RegisterOwner(owner); err != nil {
				t.Fatal(err)
			}
			begin := func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
				if o != owner {
					return nil, nil, broker.ErrAdmissionStale
				}
				ticket, err := admission.AdmitWork(o, k)
				return conversation.WithOwnership(ctx, cap), ticket, err
			}
			g := compactiongen.New(store, nil, compactor.Config{}, nil, 0)
			g.SetEnabled(true)
			g.SetToolElisionOnly(true)
			defer g.Close(context.Background())
			entered, resume := make(chan struct{}), make(chan struct{})
			g.SetElideOnlyFn(func(ctx context.Context, id string) (int, int, int, bool, error) {
				close(entered)
				select {
				case <-resume:
				case <-ctx.Done():
					return 0, 0, 0, false, ctx.Err()
				}
				err := store.SaveCompaction(ctx, conversation.Compaction{ConversationID: id, FrozenThrough: 42})
				return 20, 10, 1, true, err
			})
			g.SetContextUsageFn(func(ctx context.Context, id string, sent, raw int) {
				if fail { // A real rejected durable write, surfaced through the void callback.
					ReportWriteFailure(ctx, store.SaveContextUsage(context.Background(), conversation.ContextUsage{ConversationID: id, TokensUsed: sent}))
					return
				}
				ReportWriteFailure(ctx, store.SaveContextUsage(ctx, conversation.ContextUsage{ConversationID: id, TokensUsed: sent}))
			})
			stale := owner
			stale.Epoch = "stale"
			if _, err := ScheduleOwnedCompaction(ctx, g, stale, begin); !errors.Is(err, broker.ErrAdmissionStale) {
				t.Fatalf("stale: %v", err)
			}
			work, err := ScheduleOwnedCompaction(ctx, g, owner, begin)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if _, err := admission.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("execution gap: %v", err)
			}
			// A separate SQLite write succeeds while the producer is blocked outside SQL.
			if err := store.SaveContextUsage(owned, conversation.ContextUsage{ConversationID: "c", TokensUsed: 2}); err != nil {
				t.Fatal(err)
			}
			close(resume)
			select {
			case <-work.Done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			state, err := store.GetCompaction(owned, "c")
			if err != nil || state.FrozenThrough != 42 {
				t.Fatalf("state=%+v err=%v", state, err)
			}
			if fail {
				if work.Err == nil {
					t.Fatal("write failure hidden")
				}
				if _, err := admission.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("failure lost blocker: %v", err)
				}
			} else {
				if work.Err != nil {
					t.Fatal(work.Err)
				}
				usage, ok, err := store.GetContextUsage(owned, "c")
				if err != nil || !ok || usage.TokensUsed != 10 {
					t.Fatalf("usage=%+v err=%v", usage, err)
				}
				closure, err := admission.CloseOwner(owner, nil)
				if err != nil {
					t.Fatal(err)
				}
				_ = closure.Reopen()
			}
		})
	}
}

func TestOwnedCompactionQueuedCancellationDrain(t *testing.T) {
	ctx := context.Background()
	owner := broker.Owner{Conversation: "queued", Host: "host", Epoch: "epoch"}
	a := broker.NewAdmission("host")
	if err := a.RegisterOwner(owner); err != nil {
		t.Fatal(err)
	}
	g := compactiongen.New(nil, nil, compactor.Config{}, nil, time.Hour)
	g.SetEnabled(true)
	work, err := ScheduleOwnedCompaction(ctx, g, owner, func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
		ticket, err := a.AdmitWork(o, k)
		return ctx, ticket, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("timer gap: %v", err)
	}
	deadline, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := g.Close(deadline); err != nil {
		t.Fatal(err)
	}
	<-work.Done
	if !errors.Is(work.Err, context.Canceled) {
		t.Fatal(work.Err)
	}
	if _, err := a.CloseOwner(owner, nil); err != nil {
		t.Fatal(err)
	}
}
