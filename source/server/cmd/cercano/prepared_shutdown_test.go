package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPreparedShutdownBarrier(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	sentinel := errors.New("dispose failure")
	h := newPreparedShutdown(nil, func() error { calls.Add(1); <-release; return sentinel })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := h.Wait(ctx)
	var pending *preparedShutdownPending
	if !errors.As(err, &pending) || time.Since(start) > time.Second {
		t.Fatalf("unbounded or missing pending: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := h.Wait(ctx); !errors.As(err, &pending) {
			t.Fatalf("lost pending: %v", err)
		}
	}
	if h.Status().Complete {
		t.Fatal("barrier reported complete")
	}
	close(release)
	<-h.done
	if err := h.Wait(context.Background()); !errors.Is(err, sentinel) {
		t.Fatalf("lost error: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatal("disposal repeated")
	}
}

func TestPreparedShutdownStartupFailure(t *testing.T) {
	release := make(chan struct{})
	startup := errors.New("late construction failure")
	disposal := errors.New("close failed")
	h := newPreparedShutdown(nil, func() error { <-release; return disposal })
	err := finishPreparedShutdown(h, startup)
	var pending *preparedShutdownPending
	if !errors.Is(err, startup) || !errors.As(err, &pending) {
		t.Fatalf("lost startup/pending: %v", err)
	}
	close(release)
	err = retainPreparedShutdown(err)
	if !errors.Is(err, startup) || !errors.Is(err, disposal) {
		t.Fatalf("lost final errors: %v", err)
	}
}

func TestPreparedShutdownOwnerRetainsAfterCancellation(t *testing.T) {
	release := make(chan struct{})
	var disposed atomic.Bool
	h := newPreparedShutdown(nil, func() error { <-release; disposed.Store(true); return nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pending := h.WaitBounded(ctx)
	ownerDone := make(chan error, 1)
	go func() { ownerDone <- retainPreparedShutdown(pending) }()
	select {
	case <-ownerDone:
		t.Fatal("process owner released live callback")
	case <-time.After(20 * time.Millisecond):
	}
	if disposed.Load() {
		t.Fatal("premature disposal")
	}
	close(release)
	select {
	case err := <-ownerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("owner did not finish")
	}
}

func TestPreparedShutdownRepeatedBoundedParticipation(t *testing.T) {
	release := make(chan struct{})
	sentinel := errors.New("store close failed")
	var closes atomic.Int32
	resources := newPreparedShutdown(nil, func() error { closes.Add(1); <-release; return sentinel })
	for i := 0; i < 2; i++ {
		start := time.Now()
		var pending *preparedShutdownPending
		if err := resources.WaitBounded(context.Background()); !errors.As(err, &pending) {
			t.Fatalf("retirement succeeded: %v", err)
		}
		if time.Since(start) > time.Second {
			t.Fatal("unbounded participation close")
		}
	}
	close(release)
	<-resources.done
	if !errors.Is(resources.WaitBounded(context.Background()), sentinel) {
		t.Fatal("store error lost")
	}
	if closes.Load() != 1 {
		t.Fatal("repeated resource disposal")
	}
}

func TestPreparedShutdownControlRemainsObservable(t *testing.T) {
	release := make(chan struct{})
	sentinel := errors.New("sensitive path: fake close error")
	h := newPreparedShutdown(nil, func() error { <-release; return sentinel })
	if s := h.Control(true); s.Accepted || s.Complete {
		t.Fatal(s)
	}
	for i := 0; i < 3; i++ {
		if s := h.Control(false); !s.Accepted || s.Complete || s.Phase != "draining" {
			t.Fatal(s)
		}
	}
	close(release)
	<-h.done
	s := h.Control(true)
	if s.Phase != "failed" || s.IncompleteReason != "resource disposal failed" {
		t.Fatal(s)
	}
	if !errors.Is(h.Wait(context.Background()), sentinel) {
		t.Fatal("local error lost")
	}
}

func TestPreparedShutdownFailureNeverComplete(t *testing.T) {
	failure := errors.New("disposal failed")
	h := newPreparedShutdown(nil, func() error { return failure })
	if err := h.Wait(context.Background()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if s := h.Control(true); s.Complete || s.Phase != "failed" {
			t.Fatal(s)
		}
	}
}
