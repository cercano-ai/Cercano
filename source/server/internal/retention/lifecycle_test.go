package retention

import (
	"context"
	"testing"
	"time"
)

// Cancellation during the initial delay must join without touching the store.
// A nil store traps accidental sweeping; no database or clock sleeps are used.
func TestStartCancellationJoins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := New(nil, Config{}, time.Hour).Start(ctx)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("sweeper did not join")
	}
	cancel()
	<-done
}
