package localruntime

import (
	"context"
	"sync"
	"testing"
)

// sharedFake is a Manager shared with other processes: Start counts calls and
// returns the instance it was asked for; HoldEpoch is settable.
type sharedFake struct {
	*InMemoryManager
	mu        sync.Mutex
	starts    int
	epoch     uint64
	observers []Observer
}

func (s *sharedFake) Start(_ context.Context, req StartRequest) (*InstanceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starts++
	return &InstanceRecord{ID: "inst-" + req.ModelID, Runtime: req.Runtime, ModelID: req.ModelID, State: InstanceRunning}, nil
}

func (s *sharedFake) RegisterObserver(o Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, o)
}

func (s *sharedFake) HoldEpoch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.epoch
}

func (s *sharedFake) startCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starts
}

func (s *sharedFake) emit(ev InstanceEvent) {
	s.mu.Lock()
	obs := append([]Observer(nil), s.observers...)
	s.mu.Unlock()
	for _, o := range obs {
		o.OnInstanceStateChange(ev)
	}
}

func TestHoldTrackerTakesOneHoldPerInstance(t *testing.T) {
	m := &sharedFake{InMemoryManager: NewManager(), epoch: 1}
	var h HoldTracker
	ctx := context.Background()
	inst := InstanceRecord{ID: "inst-m", Runtime: "rt", ModelID: "m", State: InstanceRunning}

	for i := 0; i < 3; i++ {
		got, err := h.Hold(ctx, m, inst)
		if err != nil || got.ID != inst.ID {
			t.Fatalf("Hold = %+v, %v", got, err)
		}
	}
	if n := m.startCount(); n != 1 {
		t.Fatalf("Start calls = %d, want 1 (cached after the first hold)", n)
	}

	// The instance stopping drops the cached hold.
	m.emit(InstanceEvent{Instance: InstanceRecord{ID: inst.ID}, Prev: InstanceRunning, Next: InstanceStopped})
	if _, err := h.Hold(ctx, m, inst); err != nil {
		t.Fatal(err)
	}
	if n := m.startCount(); n != 2 {
		t.Fatalf("Start calls after stop event = %d, want 2", n)
	}

	// A new epoch (the manager reconnected) invalidates every cached hold.
	m.mu.Lock()
	m.epoch++
	m.mu.Unlock()
	if _, err := h.Hold(ctx, m, inst); err != nil {
		t.Fatal(err)
	}
	if n := m.startCount(); n != 3 {
		t.Fatalf("Start calls after epoch change = %d, want 3", n)
	}

	// This process releasing its hold (a Stop through the shared manager)
	// drops the cached hold even though the instance keeps running.
	h.OnHoldReleased(inst.ID)
	if _, err := h.Hold(ctx, m, inst); err != nil {
		t.Fatal(err)
	}
	if n := m.startCount(); n != 4 {
		t.Fatalf("Start calls after this process released its hold = %d, want 4", n)
	}

	// An instance this process started itself is already held.
	h.Held(m, "inst-own")
	if _, err := h.Hold(ctx, m, InstanceRecord{ID: "inst-own", Runtime: "rt", ModelID: "own", State: InstanceRunning}); err != nil {
		t.Fatal(err)
	}
	if n := m.startCount(); n != 4 {
		t.Fatalf("Start calls for an instance started by this process = %d, want 4", n)
	}
}

func TestHoldTrackerIgnoresInProcessManager(t *testing.T) {
	m := NewManager()
	var h HoldTracker
	inst := InstanceRecord{ID: "x", Runtime: "rt", ModelID: "m", State: InstanceRunning}
	got, err := h.Hold(context.Background(), m, inst)
	if err != nil || got.ID != "x" {
		t.Fatalf("Hold = %+v, %v", got, err)
	}
	// No provider "rt" is registered: a Start call would have failed.
}
