package main

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProductionIncompleteShutdownControl(t *testing.T) {
	gate, sibling := newProductionAdmissionGate(), newProductionAdmissionGate()
	release := make(chan struct{})
	var disposed atomic.Int32
	shutdown := newProductionShutdown(gate.close, func() { <-release; disposed.Add(1) })
	var wg sync.WaitGroup
	started := time.Now()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if shutdown.Shutdown().Complete {
				t.Error("reported retirement before callback completion")
			}
		}()
	}
	wg.Wait()
	if time.Since(started) > time.Second {
		t.Fatal("shutdown requests were not bounded")
	}
	if shutdown.Status().Complete || disposed.Load() != 0 {
		t.Fatal("premature disposal")
	}
	next := func(context.Context, any) (any, error) { return nil, nil }
	if _, err := gate.unary(context.Background(), nil, nil, next); status.Code(err) != codes.Unavailable {
		t.Fatal("ordinary admission remained open", err)
	}
	if _, err := sibling.unary(context.Background(), nil, nil, next); err != nil {
		t.Fatal("safe sibling fenced", err)
	}
	close(release)
	select {
	case <-shutdown.done:
	case <-time.After(time.Second):
		t.Fatal("callback did not dispose automatically")
	}
	for i := 0; i < 8; i++ {
		if !shutdown.Shutdown().Complete {
			t.Fatal("completion lost")
		}
	}
	if disposed.Load() != 1 {
		t.Fatal("disposal not exactly once")
	}
}
