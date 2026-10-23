package localruntime

import (
	"context"
	"errors"
	"sync"
)

// SharedManager is implemented by a Manager whose instances are shared with
// other processes through holds, such as a client of the shared model host.
// Such a manager stops an instance only once no process holds it:
//
//   - Start takes a hold for the caller, sharing a running instance of the
//     same model instead of launching another. A caller that found an
//     instance through Instances must Start it before using it, or another
//     process may stop it underneath.
//   - Stop releases only the caller's hold. The instance keeps running while
//     others hold it, so no stop event follows; instead, before Stop returns,
//     the manager tells each registered HoldReleaseObserver that this
//     process no longer holds the instance.
//   - Closing the manager releases every hold the caller still has, so
//     callers must not stop shared instances themselves on exit.
type SharedManager interface {
	Manager
	// HoldEpoch identifies the connection holds were taken on. It changes
	// whenever holds taken earlier may have been lost, for example after a
	// reconnect, so callers caching which instances they hold must take them
	// again.
	HoldEpoch() uint64
}

// HoldReleaseObserver is an Observer that a SharedManager also tells, before
// Stop returns, that this process released its hold on an instance. Unlike an
// instance event, this is local to the process: other holders may keep the
// instance running.
type HoldReleaseObserver interface {
	OnHoldReleased(instanceID string)
}

// HoldTracker remembers which instances of a SharedManager this process
// holds, so a caller reusing a running instance takes its hold once rather
// than on every request. The zero value is ready to use; a HoldTracker must
// not be copied after first use.
type HoldTracker struct {
	mu       sync.Mutex
	observed Manager
	held     map[string]uint64 // instance ID -> epoch the hold was taken in
}

// Hold makes sure this process holds inst before the caller uses it. For a
// manager that is not shared it does nothing and returns inst. Otherwise,
// unless inst is already held in the current epoch, it calls Start for inst's
// model: the shared manager dedupes to the running instance and records the
// hold. The returned record is the instance now held; it differs from inst
// only if inst stopped in the meantime and Start launched a replacement.
func (h *HoldTracker) Hold(ctx context.Context, m Manager, inst InstanceRecord) (InstanceRecord, error) {
	sm, ok := m.(SharedManager)
	if !ok {
		return inst, nil
	}
	epoch := sm.HoldEpoch()
	h.mu.Lock()
	h.observeLocked(m)
	e, cached := h.held[inst.ID]
	h.mu.Unlock()
	if cached && e == epoch {
		return inst, nil
	}
	got, err := m.Start(ctx, StartRequest{Runtime: inst.Runtime, ModelID: inst.ModelID})
	if err != nil {
		return InstanceRecord{}, err
	}
	if got == nil {
		return InstanceRecord{}, errors.New("runtime returned no instance")
	}
	h.mu.Lock()
	h.held[got.ID] = epoch
	h.mu.Unlock()
	return *got, nil
}

// Held records that this process holds instance id on m, after its own
// Start. It does nothing for a manager that is not shared.
func (h *HoldTracker) Held(m Manager, id string) {
	sm, ok := m.(SharedManager)
	if !ok || id == "" {
		return
	}
	epoch := sm.HoldEpoch()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.observeLocked(m)
	h.held[id] = epoch
}

// observeLocked starts tracking m: it registers h for m's instance events so
// a stopped instance's hold is forgotten. Switching managers forgets every
// hold. h.mu must be held.
func (h *HoldTracker) observeLocked(m Manager) {
	if h.observed == m && h.held != nil {
		return
	}
	h.observed = m
	h.held = map[string]uint64{}
	m.RegisterObserver(h)
}

// OnDownloadStateChange implements Observer.
func (h *HoldTracker) OnDownloadStateChange(DownloadEvent) {}

// OnHoldReleased implements HoldReleaseObserver: this process stopped the
// instance through the manager, so the next use must take a hold again.
func (h *HoldTracker) OnHoldReleased(instanceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.held, instanceID)
}

// OnInstanceStateChange implements Observer: an instance that stopped,
// crashed or failed is no longer held.
func (h *HoldTracker) OnInstanceStateChange(ev InstanceEvent) {
	switch ev.Next {
	case InstanceStopped, InstanceCrashed, InstanceFailed:
	default:
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.held, ev.Instance.ID)
}
