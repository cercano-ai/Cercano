package compactiongen

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compaction"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/contextmeter"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/llm"
)

type failingCompactionStore struct {
	*fakeStore
	fail func() error
}

func (s failingCompactionStore) SaveCompaction(ctx context.Context, c conversation.Compaction) error {
	if err := s.fail(); err != nil {
		return err
	}
	return s.fakeStore.SaveCompaction(ctx, c)
}

func regressionAdmission(t *testing.T) (broker.Owner, *broker.Admission, BeginOwnedWork) {
	t.Helper()
	o := broker.Owner{Conversation: "c", Host: "host", Epoch: "epoch"}
	a := broker.NewAdmission("host")
	if err := a.RegisterOwner(o); err != nil {
		t.Fatal(err)
	}
	return o, a, func(ctx context.Context, owner broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
		ticket, err := a.AdmitWork(owner, k)
		return ctx, ticket, err
	}
}

func TestOwnedPanicAndSaveFailureSuppressSuccess(t *testing.T) {
	for _, stage := range []string{"summary", "save", "usage", "save_error", "usage_outcome"} {
		t.Run(stage, func(t *testing.T) {
			o, a, begin := regressionAdmission(t)
			sentinel := errors.New("durable write rejected")
			fs := &fakeStore{turns: bigTurns(12, 1000)}
			store := failingCompactionStore{fs, func() error {
				if stage == "save" {
					panic("save panic")
				}
				if stage == "save_error" {
					return sentinel
				}
				return nil
			}}
			g := New(store, func(context.Context, []llm.Message) (compaction.StructuredSummary, error) {
				if stage == "summary" {
					panic("summary panic")
				}
				return compaction.StructuredSummary{Goal: "g"}, nil
			}, compactor.Config{ActivationFloorTokens: 1000, SegmentTokens: 4000, VerbatimRecent: 2}, contextmeter.Default(), 0)
			g.SetEnabled(true)
			var logs strings.Builder
			g.logf = func(f string, args ...any) { fmt.Fprintf(&logs, f, args...) }
			var outcome error
			g.SetContextUsageFn(func(context.Context, string, int, int) {
				if stage == "usage" {
					panic("usage panic")
				}
				if stage == "usage_outcome" {
					outcome = sentinel
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ctx = WithPersistenceOutcome(ctx, func() error { return outcome })
			work, err := g.ScheduleOwned(ctx, o, begin)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-work.Done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if work.Err == nil {
				t.Fatal("failure reported success")
			}
			if stage == "save_error" || stage == "usage_outcome" {
				if !errors.Is(work.Err, sentinel) {
					t.Fatalf("lost failure: %v", work.Err)
				}
			} else if !strings.Contains(work.Err.Error(), stage+" panic") {
				t.Fatal(work.Err)
			}
			if strings.Contains(logs.String(), "pass ok") {
				t.Fatalf("false success log: %s", logs.String())
			}
			if err := g.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := a.CloseOwner(o, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("drain resolved failure: %v", err)
			}
			if err := work.Ticket.Complete(); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("missing unresolved blocker: %v", err)
			}
		})
	}
}

func TestOwnedBacklogRetryTimerContinuouslyCounted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o, a, begin := regressionAdmission(t)
		fs := &fakeStore{turns: bigTurns(30, 1000)}
		g := New(fs, func(context.Context, []llm.Message) (compaction.StructuredSummary, error) {
			return compaction.StructuredSummary{Goal: "g"}, nil
		}, compactor.Config{ActivationFloorTokens: 1000, SegmentTokens: 4000, VerbatimRecent: 2}, contextmeter.Default(), time.Hour)
		g.SetEnabled(true)
		var logs strings.Builder
		g.logf = func(f string, args ...any) { fmt.Fprintf(&logs, f, args...) }
		ctx, cancel := context.WithCancel(context.Background())
		admissions := 0
		work, err := g.ScheduleOwned(ctx, o, func(ctx context.Context, owner broker.Owner, kind broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
			admissions++
			return begin(ctx, owner, kind)
		})
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		time.Sleep(time.Hour)
		synctest.Wait() // pass finished; owned worker is durably blocked on its retry timer
		if !strings.Contains(logs.String(), "more=true") {
			t.Fatalf("did not produce backlog: %s", logs.String())
		}
		select {
		case <-work.Done:
			t.Fatal("backlog dropped")
		default:
		}
		if _, err := a.CloseOwner(o, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
			t.Fatalf("retry timer gap: %v", err)
		}
		time.Sleep(30 * time.Minute)
		synctest.Wait()
		if admissions != 1 {
			t.Fatalf("backlog reacquired admission: %d", admissions)
		}
		if _, err := a.CloseOwner(o, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
			t.Fatalf("mid-retry timer gap: %v", err)
		}
		cancel()
		synctest.Wait()
		if !errors.Is(work.Err, context.Canceled) {
			t.Fatal(work.Err)
		}
		if err := g.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := a.CloseOwner(o, nil); err != nil {
			t.Fatal(err)
		}
	})
}

func TestOwnedRepeatedConcurrentScheduleCancelDrainBusyExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o, a, begin := regressionAdmission(t)
		g := New(nil, nil, compactor.Config{}, nil, time.Second)
		g.SetEnabled(true)
		g.claim(o.Conversation) // force every expired pass through the busy retry path
		g.logf = func(string, ...any) {}
		ctx, cancel := context.WithCancel(context.Background())
		const n = 32
		works := make([]*ScheduledWork, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() {
				var err error
				works[i], err = g.ScheduleOwned(ctx, o, begin)
				if err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()
		if _, err := a.CloseOwner(o, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
			t.Fatalf("busy retry gap: %v", err)
		}
		start := make(chan struct{})
		for range n {
			wg.Go(func() { <-start; cancel() })
			wg.Go(func() {
				<-start
				if err := g.Close(context.Background()); err != nil {
					t.Error(err)
				}
			})
			wg.Go(func() {
				<-start
				w, err := g.ScheduleOwned(ctx, o, begin)
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Error(err)
				}
				if w != nil {
					<-w.Done
				}
			})
		}
		close(start)
		wg.Wait()
		synctest.Wait()
		for _, w := range works {
			select {
			case <-w.Done:
			default:
				t.Fatal("undrained work")
			}
			if !errors.Is(w.Err, context.Canceled) {
				t.Fatalf("busy cancellation: %v", w.Err)
			}
		}
		g.release(o.Conversation)
		if _, err := a.CloseOwner(o, nil); err != nil {
			t.Fatal(err)
		}
	})
}
