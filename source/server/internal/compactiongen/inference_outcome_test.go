package compactiongen

import (
	"context"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/compaction"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/contextmeter"
	"cercano/source/server/internal/llm"
)

func TestOwnedInferenceOutcome(t *testing.T) {
	for _, mode := range []string{"cancel", "provider", "reported_write", "partial_save_error", "validation", "read_error", "success"} {
		t.Run(mode, func(t *testing.T) {
			o, a, begin := regressionAdmission(t)
			sentinel := errors.New("write uncertain")
			fs := &fakeStore{turns: bigTurns(12, 1000)}
			if mode == "read_error" {
				fs.turnsErr = sentinel
			}
			store := failingCompactionStore{fs, func() error {
				if mode == "partial_save_error" {
					return sentinel
				}
				return nil
			}}
			calls := 0
			g := New(store, func(ctx context.Context, _ []llm.Message) (compaction.StructuredSummary, error) {
				calls++
				switch mode {
				case "cancel", "reported_write":
					return compaction.StructuredSummary{}, context.Canceled
				case "provider":
					return compaction.StructuredSummary{}, errors.New("provider unavailable")
				case "validation":
					return compaction.StructuredSummary{}, nil
				case "partial_save_error":
					if calls > 1 {
						return compaction.StructuredSummary{}, context.Canceled
					}
				}
				return compaction.StructuredSummary{Goal: "g"}, nil
			}, compactor.Config{ActivationFloorTokens: 1000, SegmentTokens: 4000, VerbatimRecent: 2}, contextmeter.Default(), 0)
			g.SetEnabled(true)
			defer g.Close(context.Background())
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			ctx = WithPersistenceOutcome(ctx, func() error {
				if mode == "reported_write" {
					return sentinel
				}
				return nil
			})
			// Repeat under one owner: three joined inference failures must not accumulate
			// phantom tickets. Counts do not identify any historical incident ticket.
			for range 3 {
				w, err := g.ScheduleOwned(ctx, o, begin)
				if err != nil {
					t.Fatal(err)
				}
				select {
				case <-w.Done:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				fenced := mode == "reported_write" || mode == "partial_save_error" || mode == "validation" || mode == "read_error"
				snap := a.Snapshot(o)
				if fenced != (snap.UnresolvedWork > 0) {
					t.Fatalf("mode=%s snapshot=%+v err=%v", mode, snap, w.Err)
				}
				if !fenced && len(snap.Work) != 0 {
					t.Fatalf("joined work remained: %+v", snap)
				}
				if mode != "success" && w.Err == nil {
					t.Fatal("lost execution error")
				}
				if mode == "success" && w.Err != nil {
					t.Fatal(w.Err)
				}
				if fenced {
					break
				}
			}
			if mode == "cancel" || mode == "provider" {
				if fs.saved != nil {
					t.Fatal("inference failure wrote summary")
				}
				if _, err := a.CloseOwner(o, nil); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// A retry can pass the scheduler's fence check immediately before Close fences
// registration. This is not an unjoined producer or a canceled store operation.
func TestOwnedRefusedExecutionHasNoEffects(t *testing.T) {
	fs := &fakeStore{turns: bigTurns(12, 1000), turnsErr: errors.New("must not read")}
	g := New(fs, func(context.Context, []llm.Message) (compaction.StructuredSummary, error) {
		t.Fatal("must not infer")
		return compaction.StructuredSummary{}, nil
	}, compactor.Config{}, contextmeter.Default(), 0)
	if err := g.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	pass := &ownedPass{}
	err := g.runCompaction(context.WithValue(t.Context(), ownedPassKey{}, pass), "parent")
	if !errors.Is(err, context.Canceled) || !pass.executionRefused || pass.inferenceOnlyFailure || fs.saved != nil {
		t.Fatalf("err=%v pass=%+v saved=%v", err, pass, fs.saved)
	}
}
