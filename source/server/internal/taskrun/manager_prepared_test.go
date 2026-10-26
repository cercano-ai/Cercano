package taskrun

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
)

// preparedTaskStore is entirely in memory; embedded methods are deliberately
// unavailable so unexpected store calls fail the test rather than touch a DB.
type preparedTaskStore struct {
	conversation.TaskStore
	mu                        sync.Mutex
	rows                      map[string]conversation.TaskRecord
	initCalls, reconcileCalls int
	rejectFinish              bool
}

func (s *preparedTaskStore) InitTasks(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.initCalls++
	return nil
}
func (s *preparedTaskStore) ReconcileTasks(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileCalls++
	for id, row := range s.rows {
		if !row.Terminal() {
			row.Status = "interrupted"
			s.rows[id] = row
		}
	}
	return nil
}
func (s *preparedTaskStore) InsertTask(_ context.Context, r conversation.TaskRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[r.ID] = r
	return nil
}
func (s *preparedTaskStore) GetTask(_ context.Context, parent, id string) (conversation.TaskRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[id]
	if !ok || r.ParentID != parent {
		return conversation.TaskRecord{}, conversation.ErrTaskNotFound
	}
	return r, nil
}
func (s *preparedTaskStore) StartTask(_ context.Context, parent, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rows[id]
	r.Status = "running"
	s.rows[id] = r
	return nil
}
func (s *preparedTaskStore) FinishTask(_ context.Context, r conversation.TaskRecord) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rejectFinish {
		return false, nil
	}
	r.NotificationPending = true
	s.rows[r.ID] = r
	return true, nil
}

func TestRejectedTerminalTransitionSuppressesEffects(t *testing.T) {
	s := &preparedTaskStore{rows: make(map[string]conversation.TaskRecord), rejectFinish: true}
	release := make(chan struct{})
	notified := make(chan string, 1)
	m, err := New(s, func(context.Context, dispatch.Spec) (dispatch.Result, error) {
		<-release
		return dispatch.Result{}, nil
	}, func(parent string) { notified <- parent })
	if err != nil {
		t.Fatal(err)
	}
	rec, err := m.Launch(deadline(t), taskSpec())
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	wake := m.registerWaitWakeLocked(rec.ParentID)
	m.mu.Unlock()
	close(release)
	m.wg.Wait() // Includes notification callback; no timing-based absence assertion.
	select {
	case <-notified:
		t.Error("rejected terminal transition published completion")
	default:
	}
	select {
	case <-wake:
		t.Error("rejected terminal transition woke sibling waiter")
	default:
	}
	_, err = m.Wait(deadline(t), rec.ParentID, rec.ID)
	if !errors.Is(err, ErrTerminalTransitionRejected) {
		t.Errorf("rejected terminal transition: %v", err)
	}
	if !errors.Is(m.UnresolvedTerminalOutcomes(rec.ParentID)[rec.ID], ErrTerminalTransitionRejected) {
		t.Error("missing explicit unresolved outcome")
	}
	if len(m.UnresolvedTerminalOutcomes("other")) != 0 {
		t.Error("cross-parent outcome")
	}
	m.mu.Lock()
	e := m.active[rec.ID]
	m.mu.Unlock()
	if e == nil || e.persistErr == nil {
		t.Error("unresolved persistence no longer blocks quiescence")
	}
	row, _ := s.GetTask(context.Background(), rec.ParentID, rec.ID)
	if row.Status != "running" || row.NotificationPending {
		t.Fatalf("durable row changed: %+v", row)
	}
	if err := m.Close(deadline(t)); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedManagerPreservesOtherHostAndPersists(t *testing.T) {
	s := &preparedTaskStore{rows: make(map[string]conversation.TaskRecord)}
	started, release := make(chan struct{}), make(chan struct{})
	a, err := New(s, func(context.Context, dispatch.Spec) (dispatch.Result, error) {
		close(started)
		<-release
		return dispatch.Result{}, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	running, err := a.Launch(deadline(t), taskSpec())
	if err != nil {
		t.Fatal(err)
	}
	<-started
	accepted := conversation.TaskRecord{ID: "a-accepted", ParentID: "parent", Status: "accepted"}
	if err := s.InsertTask(context.Background(), accepted); err != nil {
		t.Fatal(err)
	}
	before, _ := s.GetTask(context.Background(), running.ParentID, running.ID)
	b, err := NewPrepared(s, func(context.Context, dispatch.Spec) (dispatch.Result, error) { return dispatch.Result{Text: "ok"}, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	initCalls, reconcileCalls := s.initCalls, s.reconcileCalls
	s.mu.Unlock()
	if initCalls != 1 || reconcileCalls != 1 {
		t.Fatalf("B initialized/reconciled: %d/%d", initCalls, reconcileCalls)
	}
	after, _ := s.GetTask(context.Background(), running.ParentID, running.ID)
	pending, _ := s.GetTask(context.Background(), accepted.ParentID, accepted.ID)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(accepted, pending) {
		t.Fatal("B changed A rows")
	}
	spec := taskSpec()
	spec.ConversationID = "b-parent"
	rec, err := b.Launch(deadline(t), spec)
	if err != nil {
		t.Fatal(err)
	}
	final, err := b.Wait(deadline(t), rec.ParentID, rec.ID)
	if err != nil || final.Status != "completed" || !final.NotificationPending {
		t.Fatalf("B completion: %+v %v", final, err)
	}
	close(release)
	if _, err := a.Wait(deadline(t), running.ParentID, running.ID); err != nil {
		t.Fatal(err)
	}
	for _, m := range []*Manager{a, b} {
		if err := m.Close(deadline(t)); err != nil {
			t.Fatal(err)
		}
		if len(m.UnresolvedTerminalOutcomes("")) != 0 {
			t.Fatal("successful persistence unresolved")
		}
	}
}

func TestPreparedSuccessfulTerminalTransitionPublishesEffects(t *testing.T) {
	s := &preparedTaskStore{rows: make(map[string]conversation.TaskRecord)}
	release := make(chan struct{})
	notified := make(chan string, 1)
	m, err := NewPrepared(s, func(context.Context, dispatch.Spec) (dispatch.Result, error) {
		<-release
		return dispatch.Result{Text: "done"}, nil
	}, func(parent string) { notified <- parent })
	if err != nil {
		t.Fatal(err)
	}
	rec, err := m.Launch(deadline(t), taskSpec())
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	wake := m.registerWaitWakeLocked(rec.ParentID)
	m.mu.Unlock()
	close(release)
	m.wg.Wait()
	select {
	case parent := <-notified:
		if parent != rec.ParentID {
			t.Fatal(parent)
		}
	default:
		t.Fatal("missing notification")
	}
	select {
	case <-wake:
	default:
		t.Fatal("missing wake")
	}
	final, err := m.Wait(deadline(t), rec.ParentID, rec.ID)
	if err != nil || final.Status != "completed" || !final.NotificationPending {
		t.Fatalf("completion: %+v %v", final, err)
	}
	if len(m.UnresolvedTerminalOutcomes("")) != 0 {
		t.Fatal("successful completion retained")
	}
	if s.initCalls != 0 || s.reconcileCalls != 0 {
		t.Fatal("prepared constructor mutated schema or reconciled")
	}
	if err := m.Close(deadline(t)); err != nil {
		t.Fatal(err)
	}
}
