package compactiongen

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compaction"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/contextmeter"
	"cercano/source/server/internal/llm"
)

func TestOwnedBusyRetryAndInferenceKeepOneTicket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type key struct{}
	ctx = context.WithValue(ctx, key{}, "authority metadata")
	owner := broker.Owner{Conversation: "c", Host: "host", Epoch: "epoch"}
	admission := broker.NewAdmission("host")
	if err := admission.RegisterOwner(owner); err != nil {
		t.Fatal(err)
	}
	busy, resume, inference, finish := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	fs := &fakeStore{turns: bigTurns(12, 1000)}
	g := New(fs, func(ctx context.Context, _ []llm.Message) (compaction.StructuredSummary, error) {
		if ctx.Value(key{}) != "authority metadata" {
			return compaction.StructuredSummary{}, errors.New("metadata lost")
		}
		once.Do(func() {
			close(inference)
			select {
			case <-finish:
			case <-ctx.Done():
			}
		})
		return compaction.StructuredSummary{Goal: "g"}, ctx.Err()
	}, compactor.Config{ActivationFloorTokens: 1000, SegmentTokens: 4000, VerbatimRecent: 2}, contextmeter.Default(), 0)
	g.SetEnabled(true)
	defer g.Close(context.Background())
	g.logf = func(format string, _ ...any) {
		if strings.Contains(format, "pass deferred") {
			close(busy)
			select {
			case <-resume:
			case <-ctx.Done():
			}
		}
	}
	if !g.claim("c") {
		t.Fatal("claim")
	}
	admissions := 0
	work, err := g.ScheduleOwned(ctx, owner, func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
		admissions++
		ticket, err := admission.AdmitWork(o, k)
		return ctx, ticket, err
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-busy:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := admission.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("busy gap: %v", err)
	}
	g.release("c")
	close(resume)
	select {
	case <-inference:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := admission.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("inference gap: %v", err)
	}
	close(finish)
	select {
	case <-work.Done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if work.Err != nil {
		t.Fatal(work.Err)
	}
	if admissions != 1 {
		t.Fatalf("retry reacquired authority: %d", admissions)
	}
	if fs.saved == nil {
		t.Fatal("no durable result")
	}
	if _, err := admission.CloseOwner(owner, nil); err != nil {
		t.Fatal(err)
	}
}
