package main

import (
	"sync"
	"time"
)

// productionShutdown is invocation-local. Incomplete never means retired and
// never authorizes replacement. Done closes only after dependency disposal.
type productionShutdown struct {
	once    sync.Once
	done    chan struct{}
	stop    func()
	dispose func()
}
type productionShutdownStatus struct{ Complete bool }

// stop must only perform the nonblocking admission fence; all cancellation,
// joins and resource callbacks belong in dispose, never in the bounded caller.
func newProductionShutdown(stop, dispose func()) *productionShutdown {
	return &productionShutdown{done: make(chan struct{}), stop: stop, dispose: dispose}
}
func (s *productionShutdown) Status() productionShutdownStatus {
	select {
	case <-s.done:
		return productionShutdownStatus{Complete: true}
	default:
		return productionShutdownStatus{}
	}
}
func (s *productionShutdown) Shutdown() productionShutdownStatus {
	s.once.Do(func() { s.stop(); go func() { s.dispose(); close(s.done) }() })
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-s.done:
	case <-timer.C:
	}
	return s.Status()
}

// The process owner must remain alive after an incomplete result. This wait is
// deliberately not a forced-exit deadline; callers can inspect Status meanwhile.
func retainUntilDisposed(s *productionShutdown) {
	if s != nil && !s.Shutdown().Complete {
		println("Shutdown incomplete: callbacks/resources retained; replacement not authorized")
		<-s.done
	}
}
