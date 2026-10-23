package localruntime

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"cercano/source/server/internal/updatecoord/nativeprocess"
)

// ManagedOwner is an optional operation-scoped native capture seam. Captures
// must honor context and must not synchronously tear down the owner. A nil
// acknowledgement is capture, never exit proof. Any context cause invalidates
// the entire observation. The set remains open to later supervisor births.
type ManagedOwner interface {
	WatchOwnedProcesses(context.Context, func(context.Context, nativeprocess.Identity) error) (context.Context, context.CancelFunc, error)
}

// ManagedOwnerSource is optional on managers, just as ManagedOwner is optional
// on providers. Callers requesting coverage must reject a missing capability.
type ManagedOwnerSource interface {
	ManagedOwners() (map[string]ManagedOwner, error)
}

// ManagedOwners fails closed only when observation is requested; ordinary
// providers and custom mocks need not implement ManagedOwner.
func (m *InMemoryManager) ManagedOwners() (map[string]ManagedOwner, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	owners := make(map[string]ManagedOwner, len(m.providers))
	for id, p := range m.providers {
		owner, ok := p.(ManagedOwner)
		if !ok {
			return nil, fmt.Errorf("runtime provider %s has no ownership coverage", id)
		}
		owners[id] = owner
	}
	return owners, nil
}

const captureLimit = 250 * time.Millisecond

type ownedCommand struct {
	identity nativeprocess.Identity
	err      error
	retiring bool
}
type runtimeWatch struct {
	ctx     context.Context
	abort   context.CancelCauseFunc
	capture func(context.Context, nativeprocess.Identity) error
	pending map[*exec.Cmd]chan struct{}
}

// ProcessOwner is embedded in the existing providers, not a global registry.
// Its independent lock makes snapshot, command birth and retirement atomic;
// callbacks and bounded acknowledgement waits never hold this lock.
// Zero value is ready for use.
type ProcessOwner struct {
	mu        sync.Mutex
	owned     map[*exec.Cmd]*ownedCommand
	watch     *runtimeWatch
	uncertain bool
}

// StartOwned preserves ordinary Start errors; native inspection is evidence
// only and never a prerequisite for starting on unsupported platforms.
func (o *ProcessOwner) StartOwned(cmd *exec.Cmd) error {
	o.mu.Lock()
	if err := cmd.Start(); err != nil {
		o.mu.Unlock()
		return err
	}
	identity, err := nativeprocess.Inspect(cmd.Process.Pid)
	h := &ownedCommand{identity: identity, err: err}
	if o.owned == nil {
		o.owned = make(map[*exec.Cmd]*ownedCommand)
	}
	o.owned[cmd] = h
	w := o.watch
	if w != nil && w.ctx.Err() == nil {
		w.pending[cmd] = make(chan struct{})
	} else {
		w = nil
	}
	o.mu.Unlock()
	if w != nil {
		go o.notify(w, cmd, h)
	}
	return nil
}

// UncertainOwnership records an adopted external owner, never as owned. This
// conservative coverage gap persists even if its local dashboard record drops.
func (o *ProcessOwner) UncertainOwnership() {
	o.mu.Lock()
	o.uncertain = true
	if o.watch != nil {
		o.watch.abort(errors.New("adopted runtime ownership cannot be observed"))
	}
	o.mu.Unlock()
}

func (o *ProcessOwner) WatchOwnedProcesses(ctx context.Context, capture func(context.Context, nativeprocess.Identity) error) (context.Context, context.CancelFunc, error) {
	if ctx == nil || capture == nil {
		return nil, nil, errors.New("runtime observation requires context and capture")
	}
	o.mu.Lock()
	if o.uncertain || (o.watch != nil && o.watch.ctx.Err() == nil) {
		o.mu.Unlock()
		return nil, nil, errors.New("runtime ownership uncertain or already observed")
	}
	for _, h := range o.owned {
		if h.retiring {
			o.mu.Unlock()
			return nil, nil, errors.New("runtime retirement already in progress")
		}
	}
	observed, abort := context.WithCancelCause(ctx)
	w := &runtimeWatch{ctx: observed, abort: abort, capture: capture, pending: make(map[*exec.Cmd]chan struct{})}
	o.watch = w
	initial := make(map[*exec.Cmd]*ownedCommand, len(o.owned))
	for cmd, h := range o.owned {
		initial[cmd] = h
		w.pending[cmd] = make(chan struct{})
	}
	o.mu.Unlock()
	cancel := func() { abort(context.Canceled) }
	for cmd, h := range initial {
		go o.notify(w, cmd, h)
	}
	timer := time.NewTimer(captureLimit)
	defer timer.Stop()
	for cmd := range initial {
		o.mu.Lock()
		ack := w.pending[cmd]
		o.mu.Unlock()
		select {
		case <-ack:
		case <-observed.Done():
			return observed, cancel, context.Cause(observed)
		case <-timer.C:
			abort(errors.New("runtime initial capture timed out"))
			return observed, cancel, context.Cause(observed)
		}
	}
	return observed, cancel, context.Cause(observed)
}

func (o *ProcessOwner) notify(w *runtimeWatch, cmd *exec.Cmd, h *ownedCommand) {
	o.mu.Lock()
	ack := w.pending[cmd]
	o.mu.Unlock()
	// Bound even an uncooperative subscriber without delaying admitted work.
	timer := time.AfterFunc(captureLimit, func() { w.abort(errors.New("runtime capture timed out")) })
	defer timer.Stop()
	if w.ctx.Err() != nil {
		return
	}
	if h.err != nil {
		w.abort(h.err)
		return
	}
	identity, err := nativeprocess.Inspect(h.identity.PID)
	if err == nil && !identity.SameProcess(h.identity) {
		err = errors.New("runtime identity changed before capture")
	}
	if err == nil {
		err = w.capture(w.ctx, identity)
	}
	if err != nil {
		w.abort(err)
		return
	}
	close(ack)
}

// RetireOwned bounds observation delay, not user cancellation or admitted work.
// A stop error leaves the command owned; only Wait completion removes it.
func (o *ProcessOwner) RetireOwned(cmd *exec.Cmd) {
	o.mu.Lock()
	if h := o.owned[cmd]; h != nil {
		h.retiring = true
	}
	w := o.watch
	var ack chan struct{}
	if w != nil {
		ack = w.pending[cmd]
	}
	o.mu.Unlock()
	if ack == nil {
		return
	}
	timer := time.NewTimer(captureLimit)
	defer timer.Stop()
	select {
	case <-ack:
	case <-w.ctx.Done():
	case <-timer.C:
		w.abort(errors.New("runtime capture not acknowledged before retirement"))
	}
}

func (o *ProcessOwner) ExitedOwned(cmd *exec.Cmd) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if w := o.watch; w != nil {
		if ack := w.pending[cmd]; ack != nil {
			select {
			case <-ack:
			default:
				w.abort(errors.New("runtime exited before native capture"))
			}
		}
	}
	delete(o.owned, cmd)
}
