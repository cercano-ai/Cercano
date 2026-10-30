package retention

import (
	"cercano/source/server/internal/conversation"
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type blockedStore struct {
	fakeStore
	entered, release chan struct{}
	calls            atomic.Int32
}

func (s *blockedStore) List(context.Context, string, int) ([]conversation.Info, error) {
	s.calls.Add(1)
	close(s.entered)
	<-s.release
	return s.infos, nil
}
func TestShutdownAdmissionActiveSweepJoin(t *testing.T) {
	store := &blockedStore{entered: make(chan struct{}), release: make(chan struct{}), fakeStore: fakeStore{infos: []conversation.Info{{ID: "old", LastTurnAt: time.Unix(0, 0)}}}}
	s := New(store, Config{RawRetentionDays: 1, CompactedRetentionDays: 2}, time.Hour)
	done := make(chan struct{})
	go func() { s.Sweep(context.Background(), time.Now()); close(done) }()
	<-store.entered
	s.Fence()
	s.Sweep(context.Background(), time.Now())
	joined := make(chan struct{})
	go func() { s.Wait(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("joined entered sweep")
	default:
	}
	close(store.release)
	<-done
	<-joined
	if store.calls.Load() != 1 || len(store.collapsed) != 1 {
		t.Fatalf("calls=%d collapsed=%v", store.calls.Load(), store.collapsed)
	}
}
