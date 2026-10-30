package server

import (
	"cercano/source/server/internal/conversation"
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

type retirementCountStore struct {
	conversation.Store
	closes atomic.Int32
}

func (s *retirementCountStore) Close() error { s.closes.Add(1); return s.Store.Close() }

func TestRetirementContinuesAfterCallerCancellation(t *testing.T) {
	s, _, _ := lifecycleFixture(t)
	st := &retirementCountStore{Store: s.coexistence.store}
	s.coexistence.store = st
	entered, release := make(chan struct{}), make(chan struct{})
	s.coexistence.options.CloseParticipation = func(ctx context.Context) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- s.RetireCoexistence(ctx) }()
	<-entered
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if st.closes.Load() != 0 {
		t.Fatal("store closed before participation")
	}
	close(release)
	if err := s.RetireCoexistence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.RetireCoexistence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseCoexistenceStore(); err != nil {
		t.Fatal(err)
	}
	if st.closes.Load() != 1 {
		t.Fatalf("closes=%d", st.closes.Load())
	}
}

func TestRetirementFailureRetainsOutcome(t *testing.T) {
	s, _, _ := lifecycleFixture(t)
	st := &retirementCountStore{Store: s.coexistence.store}
	s.coexistence.store = st
	failure := errors.New("dispose failed")
	calls := 0
	s.coexistence.options.CloseParticipation = func(context.Context) error { calls++; return failure }
	for i := 0; i < 3; i++ {
		if err := s.RetireCoexistence(context.Background()); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	if calls != 1 || st.closes.Load() != 1 {
		t.Fatalf("participation=%d store=%d", calls, st.closes.Load())
	}
}
