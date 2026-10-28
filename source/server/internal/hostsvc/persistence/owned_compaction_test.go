package persistence

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compaction"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/contextmeter"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/llm"
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
			var observed []error
			observerCtx := WithWriteFailure(ctx, func(err error) { observed = append(observed, err) })
			work, err := ScheduleOwnedCompaction(observerCtx, g, owner, begin)
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
				if len(observed) != 1 || !errors.Is(work.Err, observed[0]) {
					t.Fatalf("failure observer: observed=%v work=%v", observed, work.Err)
				}
				if work.Err == nil {
					t.Fatal("write failure hidden")
				}
				if _, err := admission.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("failure lost blocker: %v", err)
				}
			} else {
				if len(observed) != 0 {
					t.Fatalf("spurious failure: %v", observed)
				}
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

// Unlike the elision bridge probe, this exercises real Advance reads and saves
// around a blocked fake summary, with a separately opened SQLite writer.
func TestOwnedSummaryDoesNotHoldSQLiteWriteTransaction(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "history.db")
	if err := conversation.PrepareCoexistence(ctx, path); err != nil {
		t.Fatal(err)
	}
	first, err := conversation.OpenCoexistence(ctx, path, conversation.CurrentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := conversation.OpenCoexistence(ctx, path, conversation.CurrentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	cap, err := conversation.AcquireOwnership(ctx, first, "c", "host")
	if err != nil {
		t.Fatal(err)
	}
	defer cap.Close()
	other, err := conversation.AcquireOwnership(ctx, second, "other", "writer")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	owned := conversation.WithOwnership(ctx, cap)
	writer := conversation.WithOwnership(ctx, other)
	if err := first.EnsureConversation(owned, "c", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := second.EnsureConversation(writer, "other", "", ""); err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		if err := first.Append(owned, conversation.Turn{ID: fmt.Sprintf("t%d", i), ConversationID: "c", Role: "user", Content: strings.Repeat("lorem ipsum dolor sit amet ", 201), CreatedAt: time.Unix(int64(100+i), 0)}); err != nil {
			t.Fatal(err)
		}
	}
	owner := broker.Owner{Conversation: "c", Host: "host", Epoch: cap.Epoch()}
	a := broker.NewAdmission("host")
	if err := a.RegisterOwner(owner); err != nil {
		t.Fatal(err)
	}
	entered, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	g := compactiongen.New(first, func(ctx context.Context, _ []llm.Message) (compaction.StructuredSummary, error) {
		once.Do(func() {
			close(entered)
			select {
			case <-resume:
			case <-ctx.Done():
			}
		})
		return compaction.StructuredSummary{Goal: "g"}, ctx.Err()
	}, compactor.Config{ActivationFloorTokens: 1000, SegmentTokens: 4000, VerbatimRecent: 2}, contextmeter.Default(), 0)
	g.SetEnabled(true)
	defer func() { cancel(); _ = g.Close(context.Background()) }()
	work, err := ScheduleOwnedCompaction(ctx, g, owner, func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
		ticket, err := a.AdmitWork(o, k)
		return conversation.WithOwnership(ctx, cap), ticket, err
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := a.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("inference gap: %v", err)
	}
	// This must commit before inference is released, not merely after it finishes.
	if err := second.SaveContextUsage(writer, conversation.ContextUsage{ConversationID: "other", TokensUsed: 77}); err != nil {
		t.Fatal(err)
	}
	usage, ok, err := second.GetContextUsage(writer, "other")
	if err != nil || !ok || usage.TokensUsed != 77 {
		t.Fatalf("write not durable: %+v %v", usage, err)
	}
	close(resume)
	select {
	case <-work.Done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if work.Err != nil {
		t.Fatal(work.Err)
	}
	state, err := first.GetCompaction(owned, "c")
	if err != nil || state.FrozenThrough == 0 || state.ConsolidatedJSON == "" {
		t.Fatalf("missing saved summary: %+v %v", state, err)
	}
	if _, err := a.CloseOwner(owner, nil); err != nil {
		t.Fatal(err)
	}
}
