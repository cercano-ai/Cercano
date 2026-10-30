package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/recap"
)

// Dropping the capability forces an actual fenced SQLite rejection rather than
// returning a synthetic error. The read/inference pipeline still uses the store.
type recapWriteProbe struct {
	recap.Store
	fail string
}

func (s recapWriteProbe) UpdateRecap(ctx context.Context, id, line string) error {
	if s.fail == "recap" {
		ctx = context.Background()
	}
	bounded, cancel := BoundedWriteContext(ctx, time.Second)
	defer cancel()
	return s.Store.UpdateRecap(bounded, id, line)
}
func (s recapWriteProbe) SetGeneratedTitle(ctx context.Context, id, line string) error {
	if s.fail == "title" {
		ctx = context.Background()
	}
	return s.Store.SetGeneratedTitle(ctx, id, line)
}

func TestOwnedRecapPersistenceBridge(t *testing.T) {
	for _, mode := range []string{"durable", "recap", "title", "panic"} {
		t.Run(mode, func(t *testing.T) {
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
			second, err := conversation.OpenCoexistence(ctx, path, conversation.CurrentCoexistenceDeclaration())
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			cap, err := conversation.AcquireOwnership(ctx, store, "c", "host")
			if err != nil {
				t.Fatal(err)
			}
			defer cap.Close()
			sibling, err := conversation.AcquireOwnership(ctx, second, "sibling", "other")
			if err != nil {
				t.Fatal(err)
			}
			defer sibling.Close()
			owned, other := conversation.WithOwnership(ctx, cap), conversation.WithOwnership(ctx, sibling)
			if err := store.EnsureConversation(owned, "c", "", ""); err != nil {
				t.Fatal(err)
			}
			if err := second.EnsureConversation(other, "sibling", "", ""); err != nil {
				t.Fatal(err)
			}
			if err := store.Append(owned, conversation.Turn{ID: "t", ConversationID: "c", Role: "user", Content: "hello", CreatedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			owner := broker.Owner{Conversation: "c", Host: "host", Epoch: cap.Epoch()}
			a := broker.NewAdmission("host")
			if err := a.RegisterOwner(owner); err != nil {
				t.Fatal(err)
			}
			begin := func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
				if o != owner {
					return nil, nil, broker.ErrAdmissionStale
				}
				ticket, err := a.AdmitWork(o, k)
				return conversation.WithOwnership(ctx, cap), ticket, err
			}
			entered, resume := make(chan struct{}), make(chan struct{})
			calls := 0
			g := recap.New(recapWriteProbe{store, mode}, func(ctx context.Context, _ string) (string, error) {
				calls++
				if calls == 1 {
					close(entered)
					select {
					case <-resume:
					case <-ctx.Done():
						return "", ctx.Err()
					}
				}
				if mode == "panic" {
					panic("inference")
				}
				return "Completed Work", nil
			}, 0, 10)
			defer func() { cancel(); _ = g.Close(context.Background()) }()
			stale := owner
			stale.Epoch = "stale"
			if _, err := ScheduleOwnedRecap(ctx, g, stale, begin); !errors.Is(err, broker.ErrAdmissionStale) {
				t.Fatalf("stale: %v", err)
			}
			foreign := owner
			foreign.Conversation = "sibling"
			if _, err := ScheduleOwnedRecap(ctx, g, foreign, begin); !errors.Is(err, broker.ErrAdmissionStale) {
				t.Fatalf("sibling: %v", err)
			}
			work, err := ScheduleOwnedRecap(ctx, g, owner, begin)
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
			// A separate connection commits while inference is blocked: no transaction
			// or sibling authority is held by the summarizer.
			if err := second.UpdateRecap(other, "sibling", "independent"); err != nil {
				t.Fatal(err)
			}
			close(resume)
			select {
			case <-work.Done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			info, err := store.Get(owned, "c")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "durable" {
				if work.Err != nil || info.Recap != "Completed Work" {
					t.Fatalf("%+v %v", info, work.Err)
				}
				if _, err := a.CloseOwner(owner, nil); err != nil {
					t.Fatal(err)
				}
			} else {
				if work.Err == nil {
					t.Fatal("failure hidden")
				}
				if _, err := a.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("failure blocker: %v", err)
				}
				if (mode == "recap" || mode == "panic") && (calls != 1 || info.Recap != "") {
					t.Fatalf("dependent publication: calls=%d %+v", calls, info)
				}
			}
			sib, err := second.Get(other, "sibling")
			if err != nil || sib.Recap != "independent" {
				t.Fatalf("sibling changed: %+v %v", sib, err)
			}
		})
	}
}

func TestOwnedRecapConcurrentScheduleDrain(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	owner := broker.Owner{Conversation: "c", Host: "host", Epoch: "epoch"}
	a := broker.NewAdmission("host")
	if err := a.RegisterOwner(owner); err != nil {
		t.Fatal(err)
	}
	begin := func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
		ticket, err := a.AdmitWork(o, k)
		return ctx, ticket, err
	}
	g := recap.New(nil, nil, time.Hour, 10)
	first, err := ScheduleOwnedRecap(context.Background(), g, owner, begin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("timer gap: %v", err)
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, err := ScheduleOwnedRecap(context.Background(), g, owner, begin)
			if err == nil {
				<-w.Done
				if !errors.Is(w.Err, context.Canceled) {
					t.Errorf("work: %v", w.Err)
				}
			} else if !errors.Is(err, context.Canceled) {
				t.Errorf("schedule: %v", err)
			}
		}()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	g.Fence()
	if err := g.Close(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	<-first.Done
	if _, err := a.CloseOwner(owner, nil); err != nil {
		t.Fatal(err)
	}
}
