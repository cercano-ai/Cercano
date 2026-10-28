package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/taskrun"
)

// A second caller must join the original executor, not the now-hidden manager.
func TestTaskShutdownConcurrentJoinInsensitiveExecutor(t *testing.T) {
	s, _ := newTestServer()
	ts := openServerTaskStore(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	m, err := taskrun.New(ts, func(context.Context, dispatch.Spec) (dispatch.Result, error) {
		close(entered)
		<-release
		return dispatch.Result{}, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.taskManager = m
	if _, err := m.Launch(context.Background(), dispatch.Spec{ConversationID: "join", Task: "block"}); err != nil {
		t.Fatal(err)
	}
	<-entered
	first := make(chan struct{})
	go func() { s.closeTaskManager(); close(first) }()
	deadline := time.After(time.Second)
	for {
		s.taskRunMu.RLock()
		closed := s.taskManagerClosed
		s.taskRunMu.RUnlock()
		if closed {
			break
		}
		select {
		case <-deadline:
			t.Fatal("admission not closed")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	second := make(chan struct{})
	go func() { s.closeTaskManager(); close(second) }()
	select {
	case <-second:
		t.Error("second shutdown falsely completed while executor still running")
	case <-time.After(50 * time.Millisecond):
	}
	// Release before joining so the test never tears persistence down under work.
	unblock()
	for _, done := range []chan struct{}{first, second} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("join did not complete")
		}
	}
}

// A timed-out caller retains the same join until insensitive completion work exits.
func TestTaskShutdownDeadlineRetainsCompletionJoin(t *testing.T) {
	s, _ := newTestServer()
	sibling, _ := newTestServer()
	s.taskWakeWG.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := s.closeTaskManagerContext(ctx); err != context.DeadlineExceeded {
		t.Fatalf("wait error = %v", err)
	}
	select {
	case <-s.taskCloseDone:
		t.Fatal("completion barrier was discarded")
	default:
	}
	sibling.taskRunMu.RLock()
	fenced := sibling.taskManagerClosed
	sibling.taskRunMu.RUnlock()
	if fenced {
		t.Fatal("safe sibling fenced")
	}
	s.taskWakeWG.Done()
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := s.closeTaskManagerContext(ctx2); err != nil {
		t.Fatal(err)
	}
}
