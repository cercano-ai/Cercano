package main

import (
	"cercano/source/server/pkg/proto"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Invocation-local ownership: a pending result never grants retirement or
// replacement. The process owner must retain the returned error's handle.
type preparedShutdown struct {
	requested atomic.Bool
	once      sync.Once
	done      chan struct{}
	fence     func()
	dispose   func() error
	err       error // published by closing done
}
type preparedShutdownStatus struct {
	Complete bool
	Err      error
}
type preparedShutdownPending struct{ Handle *preparedShutdown }

func (e *preparedShutdownPending) Error() string {
	return "prepared shutdown incomplete: resources retained; replacement not authorized"
}
func newPreparedShutdown(fence func(), dispose func() error) *preparedShutdown {
	return &preparedShutdown{done: make(chan struct{}), fence: fence, dispose: dispose}
}
func (h *preparedShutdown) Status() preparedShutdownStatus {
	select {
	case <-h.done:
		return preparedShutdownStatus{h.err == nil, h.err}
	default:
		return preparedShutdownStatus{}
	}
}
func (h *preparedShutdown) Wait(ctx context.Context) error {
	h.Request()
	select {
	case <-h.done:
		return h.err
	default:
	}
	select {
	case <-h.done:
		return h.err
	case <-ctx.Done():
		return &preparedShutdownPending{h}
	}
}
func finishPreparedShutdown(h *preparedShutdown, cause error) error {
	if h == nil {
		return cause
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	return errors.Join(cause, h.Wait(ctx))
}

// Main dispatch must call this before returning/exiting, including startup
// failure. This is intentionally an unbounded process-lifetime join, not a
// forced-exit deadline. Status remains independently observable while waiting.
func retainPreparedShutdown(err error) error {
	if p, ok := err.(*preparedShutdownPending); ok {
		<-p.Handle.done
		return p.Handle.Status().Err
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var result error
		for _, child := range joined.Unwrap() {
			result = errors.Join(result, retainPreparedShutdown(child))
		}
		return result
	}
	return err
}

func (h *preparedShutdown) Request() {
	h.once.Do(func() {
		if h.fence != nil {
			h.fence()
		}
		h.requested.Store(true)
		go func() { h.err = h.dispose(); close(h.done) }()
	})
}

func (h *preparedShutdown) Control(observe bool) *proto.ShutdownAgentResponse {
	if !observe {
		h.Request()
	}
	st := h.Status()
	r := &proto.ShutdownAgentResponse{Accepted: h.requested.Load(), Complete: st.Complete, Phase: "running"}
	if r.Accepted {
		r.Phase = "draining"
		r.IncompleteReason = "callbacks or resource disposal pending"
	}
	if st.Complete || st.Err != nil {
		r.Phase = "complete"
		r.IncompleteReason = ""
		if st.Err != nil {
			r.Phase = "failed"
			r.IncompleteReason = "resource disposal failed"
		}
	}
	return r
}

func (h *preparedShutdown) WaitBounded(ctx context.Context) error {
	bounded, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	return h.Wait(bounded)
}
