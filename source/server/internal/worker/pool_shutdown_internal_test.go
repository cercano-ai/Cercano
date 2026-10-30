package worker

import (
	"context"
	"testing"
)

func TestPoolShutdownRejectsAcquire(t *testing.T) {
	spawns := 0
	p := newWorkerPool(func(context.Context, string, uint64) (*workerHandle, error) {
		spawns++
		return &workerHandle{}, nil
	})
	p.Shutdown()
	h, err := p.Acquire(context.Background(), "after", 1)
	if h != nil || err == nil || spawns != 0 {
		t.Fatalf("Acquire after shutdown: handle=%v err=%v spawns=%d; want nil, error, 0", h, err, spawns)
	}
}

func TestPoolShutdownRejectsInflightSpawn(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	killed := false
	p := newWorkerPool(func(context.Context, string, uint64) (*workerHandle, error) {
		close(started)
		<-finish
		return &workerHandle{onKill: func() { killed = true }}, nil
	})
	result := make(chan error, 1)
	go func() {
		_, err := p.Acquire(context.Background(), "inflight", 1)
		result <- err
	}()
	<-started
	p.Shutdown()
	close(finish)
	err := <-result
	if err == nil || !killed || len(p.byConv) != 0 {
		t.Fatalf("inflight spawn after shutdown: err=%v killed=%v cached=%d; want error, true, 0", err, killed, len(p.byConv))
	}
}
