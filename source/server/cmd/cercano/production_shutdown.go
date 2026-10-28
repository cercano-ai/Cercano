package main

import (
	"cercano/source/server/pkg/proto"
	"sync"
	"sync/atomic"
	"time"
)

// productionShutdown is invocation-local. Incomplete never means retired and
// never authorizes replacement. Done closes only after dependency disposal.
type productionShutdown struct {
	requested atomic.Bool
	once      sync.Once
	done      chan struct{}
	stop      func()
	dispose   func() error
	err       error
}
type productionShutdownStatus struct {
	Complete, Pending, Failed bool
	Err                       error
}

// stop must only perform the nonblocking admission fence; all cancellation,
// joins and resource callbacks belong in dispose, never in the bounded caller.
func newProductionShutdown(stop func(), dispose func() error) *productionShutdown {
	return &productionShutdown{done: make(chan struct{}), stop: stop, dispose: dispose}
}
func (s *productionShutdown) Status() productionShutdownStatus {
	select {
	case <-s.done:
		return productionShutdownStatus{Complete: s.err == nil, Failed: s.err != nil, Err: s.err}
	default:
		return productionShutdownStatus{Pending: true}
	}
}
func (s *productionShutdown) Shutdown() productionShutdownStatus {
	s.Request()
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
	if s == nil {
		return
	}
	if s.Shutdown().Pending {
		println("Shutdown incomplete: callbacks/resources retained; replacement not authorized")
		<-s.done
	}
	if s.Status().Failed {
		println("Shutdown failed: resource disposal failed; replacement not authorized")
	}
}

// Request fences synchronously; acknowledgement never implies completion.
func (s *productionShutdown) Request() {
	s.once.Do(func() { s.stop(); s.requested.Store(true); go func() { s.err = s.dispose(); close(s.done) }() })
}
func (s *productionShutdown) Control(observe bool) *proto.ShutdownAgentResponse {
	if !observe {
		s.Request()
	}
	r := &proto.ShutdownAgentResponse{Accepted: s.requested.Load(), Complete: s.Status().Complete, Phase: "running"}
	if r.Accepted {
		r.Phase = "draining"
		r.IncompleteReason = "callbacks or resource disposal pending"
	}
	if s.Status().Failed {
		r.Phase = "failed"
		r.IncompleteReason = "resource disposal failed; replacement not authorized"
	}
	if r.Complete {
		r.Phase = "complete"
		r.IncompleteReason = ""
	}
	return r
}

// Retains the immutable startup cause and eventual rollback errors without
// mutating an error already returned to a caller. Control status never exposes it.
type productionStartupFailure struct {
	cause   error
	cleanup *productionShutdown
}

func (e *productionStartupFailure) Error() string   { return e.cause.Error() }
func (e *productionStartupFailure) Unwrap() []error { return []error{e.cause, e.cleanup.Status().Err} }
