package compactiongen

import (
	"context"
	"testing"
	"time"

	"cercano/source/server/internal/compactor"
)

// Compatibility probe: the ID-only API cannot carry parent metadata and its
// timer is not work. Keep this test as evidence, not as the owned API contract.
func TestLegacyScheduleContextLossAndTimerGap(t *testing.T) {
	type key struct{}
	parent, cancel := context.WithCancel(context.WithValue(t.Context(), key{}, "capability"))
	cancel()
	_ = parent // Schedule has no context parameter.
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	g := New(nil, nil, compactor.Config{}, nil, time.Hour)
	g.SetEnabled(true)
	g.SetToolElisionOnly(true)
	g.SetElideOnlyFn(func(ctx context.Context, _ string) (int, int, int, bool, error) {
		entered <- ctx
		<-release
		return 0, 0, 0, false, nil
	})
	defer func() {
		close(release)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := g.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	g.Schedule("c")
	g.workers.Wait() // deterministic: queued timer has no worker registration
	g.mu.Lock()
	timer := g.timers["c"]
	g.mu.Unlock()
	if timer == nil {
		t.Fatal("missing queued timer")
	}
	timer.Reset(0)
	select {
	case ctx := <-entered:
		if ctx.Value(key{}) != nil || ctx.Err() != nil {
			t.Fatal("legacy behavior changed")
		}
	case <-time.After(time.Second):
		t.Fatal("timer did not fire")
	}
}
