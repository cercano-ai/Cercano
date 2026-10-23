package worker

import (
	"context"
	"errors"
	"time"

	"cercano/source/server/internal/updatecoord/nativeprocess"
)

// WorkerOwner is the worker runner's operation-scoped owner snapshot/watch seam.
// The callback acknowledges native capture, not merely receipt of a PID. It must
// honor ctx, must not call worker teardown synchronously, and may run concurrently.
// All initial owned children and subsequent starts are delivered exactly once.
// A nil callback result means the subscriber captured its own native evidence;
// neither this API nor successful worker teardown establishes exit proof.
// Cancel the returned context to release the operation via the supplied cancel.
// Any non-nil context.Cause makes the entire observation unusable for publication.
// Final owner Shutdown closes starts; until then this is an open, not final, set.
type WorkerOwner interface {
	WatchOwnedWorkers(context.Context, func(context.Context, nativeprocess.Identity) error) (context.Context, context.CancelFunc, error)
}

func (w *workerRunner) WatchOwnedWorkers(ctx context.Context, capture func(context.Context, nativeprocess.Identity) error) (context.Context, context.CancelFunc, error) {
	if w.pool == nil {
		return nil, nil, errors.New("worker owner unavailable")
	}
	return w.pool.watchOwned(ctx, capture)
}

type workerStartKey struct{}
type workerWatch struct {
	ctx     context.Context
	abort   context.CancelCauseFunc
	capture func(context.Context, nativeprocess.Identity) error
	pending map[*workerHandle]chan struct{} // guarded by pool.mu
}

// Retirement is ordinary owner behavior, including user cancellation. Refuse
// observation after this short bound rather than holding cancellation hostage.
const workerCaptureRetirementLimit = 250 * time.Millisecond

func (p *workerPool) watchOwned(ctx context.Context, capture func(context.Context, nativeprocess.Identity) error) (context.Context, context.CancelFunc, error) {
	if ctx == nil || capture == nil {
		return nil, nil, errors.New("worker observation requires context and capture")
	}
	p.mu.Lock()
	if p.closed || (p.watch != nil && p.watch.ctx.Err() == nil) {
		p.mu.Unlock()
		return nil, nil, errors.New("worker owner closed or already observed")
	}
	for _, retiring := range p.owned {
		if retiring {
			p.mu.Unlock()
			return nil, nil, errors.New("worker retirement already in progress; capture unprovable")
		}
	}
	observed, abort := context.WithCancelCause(ctx)
	watch := &workerWatch{ctx: observed, abort: abort, capture: capture, pending: make(map[*workerHandle]chan struct{})}
	p.watch = watch
	initial := make([]*workerHandle, 0, len(p.owned))
	for h := range p.owned {
		watch.pending[h] = make(chan struct{})
		initial = append(initial, h)
	}
	p.mu.Unlock()
	for _, h := range initial {
		go watch.notify(h)
	}
	// Return only after the atomic initial snapshot has been acknowledged.
	timer := time.NewTimer(workerCaptureRetirementLimit)
	defer timer.Stop()
	for _, h := range initial {
		p.mu.Lock()
		ack := watch.pending[h]
		p.mu.Unlock()
		select {
		case <-ack:
		case <-observed.Done():
			return observed, func() { abort(context.Canceled) }, context.Cause(observed)
		case <-timer.C:
			abort(errors.New("worker initial snapshot capture timed out"))
			return observed, func() { abort(context.Canceled) }, context.Cause(observed)
		}
	}
	return observed, func() { abort(context.Canceled) }, nil
}

func (p *workerPool) own(h *workerHandle) {
	if h == nil {
		return
	}
	p.mu.Lock()
	if _, exists := p.owned[h]; exists {
		p.mu.Unlock()
		return
	}
	h.owner = p
	if h.cmd != nil && h.cmd.Process != nil {
		h.birth, h.birthErr = nativeprocess.Inspect(h.cmd.Process.Pid)
	} else {
		h.birthErr = errors.New("owned worker has no native process identity")
	}
	p.owned[h] = false
	watch := p.watch
	if watch != nil && watch.ctx.Err() == nil {
		watch.pending[h] = make(chan struct{})
	} else {
		watch = nil
	}
	p.mu.Unlock()
	if watch != nil {
		go watch.notify(h)
	}
}

func (w *workerWatch) notify(h *workerHandle) {
	h.owner.mu.Lock()
	done := w.pending[h]
	h.owner.mu.Unlock()
	if w.ctx.Err() != nil {
		return
	}
	if h.birthErr != nil {
		w.abort(h.birthErr)
		return
	}
	// Revalidate: a natural early exit is unprovable, not captured evidence.
	identity, err := nativeprocess.Inspect(h.birth.PID)
	if err == nil && !identity.SameProcess(h.birth) {
		err = errors.New("owned worker identity changed before capture")
	}
	if err == nil {
		err = w.capture(w.ctx, identity)
	}
	if err != nil {
		w.abort(err)
		return
	}

	close(done)
}

func (p *workerPool) retire(h *workerHandle) {
	p.mu.Lock()
	p.owned[h] = true
	watch := p.watch
	var ack chan struct{}
	if watch != nil {
		ack = watch.pending[h]
	}
	p.mu.Unlock()
	if ack == nil {
		return
	}
	timer := time.NewTimer(workerCaptureRetirementLimit)
	defer timer.Stop()
	select {
	case <-ack:
	case <-watch.ctx.Done():
	case <-timer.C:
		watch.abort(errors.New("worker native capture not acknowledged before bounded retirement"))
	}
}

func (p *workerPool) retired(h *workerHandle) {
	p.mu.Lock()
	delete(p.owned, h)
	// Pending acknowledgements belong to the operation; late callbacks may
	// still be completing after cancellation.
	p.mu.Unlock()
}
