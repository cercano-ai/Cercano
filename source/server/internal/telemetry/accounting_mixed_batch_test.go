package telemetry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/usage"
)

// The worker's attempt and health receipts are independent even when the
// collector flushes both records in one batch.
type mixedBatchStore struct {
	*SQLiteStore
	attemptWrites int
	healthWrites  int
	healthEntered chan struct{}
	healthRelease chan struct{}
	healthOnce    sync.Once
}

func (s *mixedBatchStore) WriteAttempts(ctx context.Context, attempts []usage.AttemptObservation) error {
	s.attemptWrites++
	return s.SQLiteStore.WriteAttempts(ctx, attempts)
}

func (s *mixedBatchStore) WriteAccountingHealth(ctx context.Context, writer string, h AccountingHealth) error {
	if writer == "worker/process/writer" {
		s.healthWrites++
		if s.healthEntered != nil {
			s.healthOnce.Do(func() { close(s.healthEntered) })
			<-s.healthRelease
		}
		return errors.New("injected remote health persistence failure")
	}
	return s.SQLiteStore.WriteAccountingHealth(ctx, writer, h)
}

func TestMixedBatchAttemptReceiptDoesNotWaitForHealthRetry(t *testing.T) {
	store := &mixedBatchStore{SQLiteStore: accountingTestStore(t), healthEntered: make(chan struct{}), healthRelease: make(chan struct{})}
	opts := testAccountingOptions()
	opts.Capacity = 4
	opts.BatchSize = 4
	opts.FlushInterval = time.Hour
	c := NewAccountingCollector(store, opts)
	attemptReceipt, err := c.TryBatch([]usage.AttemptObservation{accountingFixture("mixed-early")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.TryRemoteHealth("worker/process/writer", AccountingHealth{Sequence: 1}); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		closed <- c.Close(ctx)
	}()
	select {
	case <-store.healthEntered:
	case <-time.After(time.Second):
		t.Fatal("remote health write did not start")
	}
	select {
	case committed := <-attemptReceipt:
		if !committed {
			t.Fatal("committed attempt receipt failed")
		}
	default:
		t.Fatal("committed attempt receipt waited for independent health write")
	}
	close(store.healthRelease)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}

func TestMixedBatchAttemptCommitSurvivesRemoteHealthFailure(t *testing.T) {
	store := &mixedBatchStore{SQLiteStore: accountingTestStore(t)}
	opts := testAccountingOptions()
	opts.Capacity = 4
	opts.BatchSize = 4
	opts.FlushInterval = time.Hour // Close deterministically drains both queued records together.
	c := NewAccountingCollector(store, opts)
	attemptReceipt, err := c.TryBatch([]usage.AttemptObservation{accountingFixture("mixed-committed")})
	if err != nil {
		t.Fatal(err)
	}
	healthReceipt, err := c.TryRemoteHealth("worker/process/writer", AccountingHealth{Sequence: 1, Lost: 1})
	if err != nil {
		t.Fatal(err)
	}
	drainAccounting(t, c)
	if _, err := store.AccountingAttempt(t.Context(), "mixed-committed"); err != nil {
		t.Fatalf("committed attempt absent: %v", err)
	}
	if !<-attemptReceipt {
		t.Fatal("committed attempt was denied its persistence receipt")
	}
	if <-healthReceipt {
		t.Fatal("failed health record was acknowledged")
	}
	if got := c.Health(); got.Persisted != 1 || got.Uncertain != 1 || got.Pending != 0 {
		t.Fatalf("mixed durability misclassified: %+v", got)
	}
	if store.attemptWrites != 1 || store.healthWrites != 2 {
		t.Fatalf("unexpected retry scope: attempts=%d health=%d", store.attemptWrites, store.healthWrites)
	}
}
