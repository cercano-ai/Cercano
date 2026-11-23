package worker

import (
	"context"
	"sync"
)

type retirementDisposition uint8

const (
	keepCandidate retirementDisposition = iota
	ordinaryRetire
	unsafeRetire
)

// One owner per acquired handle. Unsafe escalation interrupts an ordinary drain;
// callback joins are deliberately outside this transport/process lifetime.
type turnRetirement struct {
	mu          sync.Mutex
	disposition retirementDisposition
	started     bool
	done        chan struct{}
	cancel      context.CancelFunc
	detach      func()
	kill        func()
	drain       func(context.Context) error
}

func (r *turnRetirement) retire(d retirementDisposition) {
	r.mu.Lock()
	if d > r.disposition {
		r.disposition = d
	}
	if r.started {
		if d == unsafeRetire {
			r.cancel()
			r.kill()
		}
		r.mu.Unlock()
		return
	}
	r.started = true
	ctx, cancel := context.WithTimeout(context.Background(), workerAccountingDrainBudget)
	r.cancel = cancel
	r.detach()
	unsafe := r.disposition == unsafeRetire
	if unsafe {
		r.kill()
	}
	r.mu.Unlock()
	go func() {
		defer close(r.done)
		defer cancel()
		if !unsafe && r.drain != nil {
			drained := make(chan struct{})
			go func() { _ = r.drain(ctx); close(drained) }()
			select {
			case <-drained:
			case <-ctx.Done():
			}
		}
		r.kill()
	}()
}

func (r *turnRetirement) finish(keep bool, release func()) {
	r.mu.Lock()
	if !r.started && keep {
		release()
		r.mu.Unlock()
		return
	}
	started := r.started
	r.mu.Unlock()
	if !started {
		r.retire(unsafeRetire)
	}
	<-r.done
}
