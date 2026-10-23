package worker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/nativeprocess"
)

func TestOwnedObservationChild(t *testing.T) {
	if os.Getenv("CERCANO_OWNED_OBSERVATION_CHILD") != "1" {
		return
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

func observationChild(t *testing.T) *workerHandle {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnedObservationChild$")
	cmd.Env = append(os.Environ(), "CERCANO_OWNED_OBSERVATION_CHILD=1")
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	h := &workerHandle{cmd: cmd}
	t.Cleanup(h.Kill)
	return h
}

func TestOwnedSnapshotInflightDisplacedAndLater(t *testing.T) {
	p := newWorkerPool(func(ctx context.Context, _ string, _ uint64) (*workerHandle, error) {
		h := observationChild(t)
		ctx.Value(workerStartKey{}).(func(*workerHandle))(h)
		return h, nil
	})
	defer p.Shutdown()
	first, err := p.Acquire(context.Background(), "same", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.Acquire(context.Background(), "same", 2)
	if err != nil {
		t.Fatal(err)
	}
	p.Release("same", second, true) // idle warm child remains relevant
	// A partial start, not yet inserted into byConv (also the partial-failure path).
	partial := observationChild(t)
	p.own(partial)
	var mu sync.Mutex
	seen := map[int]int{}
	observed, cancel, err := p.watchOwned(context.Background(), func(_ context.Context, id nativeprocess.Identity) error {
		// Reentrancy into owner mutex proves callback is outside that mutex.
		p.mu.Lock()
		p.mu.Unlock()
		mu.Lock()
		seen[id.PID]++
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	later, err := p.Acquire(context.Background(), "later", 3)
	if err != nil {
		t.Fatal(err)
	}
	partial.Kill()
	p.Release("same", first, true)
	p.Release("same", second, true)
	p.Release("later", later, false)
	p.Shutdown()
	if err := context.Cause(observed); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, h := range []*workerHandle{first, second, partial, later} {
		if seen[h.cmd.Process.Pid] != 1 {
			t.Fatalf("PID %d observed %d times", h.cmd.Process.Pid, seen[h.cmd.Process.Pid])
		}
	}
}

func TestOwnedCancellationReleasesRetirement(t *testing.T) {
	p := newWorkerPool(nil)
	entered := make(chan struct{})
	observed, cancel, err := p.watchOwned(context.Background(), func(ctx context.Context, _ nativeprocess.Identity) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	h := observationChild(t)
	p.own(h)
	<-entered
	done := make(chan struct{})
	go func() { h.Kill(); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not release retirement")
	}
	if context.Cause(observed) == nil {
		t.Fatal("cancelled observation is not proof")
	}
}

func TestOwnedUnknownIsNotProof(t *testing.T) {
	p := newWorkerPool(nil)
	h := &workerHandle{}
	p.own(h)
	defer h.Kill()
	_, cancel, err := p.watchOwned(context.Background(), func(context.Context, nativeprocess.Identity) error { t.Error("unknown identity delivered"); return nil })
	if cancel != nil {
		defer cancel()
	}
	if err == nil {
		t.Fatal("unknown identity accepted")
	}
}

func TestShutdownRacingAcquireInsertion(t *testing.T) {
	started, finish := make(chan struct{}), make(chan struct{})
	killed := make(chan struct{})
	p := newWorkerPool(func(context.Context, string, uint64) (*workerHandle, error) {
		close(started)
		<-finish
		return &workerHandle{onKill: func() { close(killed) }}, nil
	})
	acquired := make(chan error, 1)
	go func() { _, err := p.Acquire(context.Background(), "race", 1); acquired <- err }()
	<-started
	stopped := make(chan struct{})
	go func() { p.Shutdown(); close(stopped) }()
	<-p.done // shutdown has atomically closed starts
	if _, err := p.Acquire(context.Background(), "closed", 2); err == nil {
		t.Fatal("start after close")
	}
	close(finish)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("shutdown deadlocked")
	}
	if err := <-acquired; err == nil {
		t.Fatal("Acquire inserted after Shutdown")
	}
	select {
	case <-killed:
	default:
		t.Fatal("late handle leaked")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.byConv) != 0 || len(p.owned) != 0 {
		t.Fatal("shutdown retained children")
	}
}

func TestOwnedBoundedRefusalDoesNotCancelWork(t *testing.T) {
	p := newWorkerPool(nil)
	entered, release := make(chan struct{}), make(chan struct{})
	observed, cancel, err := p.watchOwned(context.Background(), func(context.Context, nativeprocess.Identity) error {
		close(entered)
		<-release
		return errors.New("late acknowledgement")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	defer close(release)
	h := observationChild(t)
	p.own(h)
	<-entered
	// The observer cannot kill a worker; only this ordinary owner retirement does.
	h.Kill()
	if context.Cause(observed) == nil {
		t.Fatal("unacknowledged retirement accepted")
	}
}

func TestOwnedPartialStartFailure(t *testing.T) {
	var child *workerHandle
	p := newWorkerPool(func(ctx context.Context, _ string, _ uint64) (*workerHandle, error) {
		child = observationChild(t)
		ctx.Value(workerStartKey{}).(func(*workerHandle))(child)
		child.Kill() // e.g. socket readiness failed, before Acquire insertion
		return nil, errors.New("readiness failed")
	})
	captured := make(chan int, 1)
	observed, cancel, err := p.watchOwned(context.Background(), func(_ context.Context, id nativeprocess.Identity) error {
		captured <- id.PID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if _, err := p.Acquire(context.Background(), "partial", 1); err == nil {
		t.Fatal("expected partial failure")
	}
	if context.Cause(observed) != nil {
		t.Fatal(context.Cause(observed))
	}
	select {
	case pid := <-captured:
		if pid != child.cmd.Process.Pid {
			t.Fatal("wrong child")
		}
	default:
		t.Fatal("partial start omitted")
	}
	p.Shutdown()
}

func TestOwnedNaturalExitBeforeCaptureIsUnprovable(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	h := &workerHandle{cmd: cmd}
	p := newWorkerPool(nil)
	p.own(h)
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	_, cancel, err := p.watchOwned(context.Background(), func(context.Context, nativeprocess.Identity) error {
		return errors.New("native evidence unavailable for exited child")
	})
	if cancel != nil {
		defer cancel()
	}
	if err == nil {
		t.Fatal("natural exit treated as proof")
	}
	// Already waited; remove owner bookkeeping without attempting another wait.
	p.retired(h)
}
