package server

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"time"
)

// Gates serialize journals, not ownership authority. refs includes holder and waiters.
type resumeGate struct {
	token chan struct{}
	refs  int
}

func (l *coexistenceLifecycle) enterResume(ctx context.Context, id string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.resumeGateMu.Lock()
	if l.resumeGates == nil {
		l.resumeGates = make(map[string]*resumeGate)
	}
	g := l.resumeGates[id]
	if g == nil {
		g = &resumeGate{token: make(chan struct{}, 1)}
		g.token <- struct{}{}
		l.resumeGates[id] = g
	}
	g.refs++
	l.resumeGateMu.Unlock()
	drop := func() {
		l.resumeGateMu.Lock()
		defer l.resumeGateMu.Unlock()
		g.refs--
		if g.refs == 0 && l.resumeGates[id] == g {
			delete(l.resumeGates, id)
		}
	}
	select {
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	case <-g.token:
	}
	release := func() { g.token <- struct{}{}; drop() }
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

// Callbacks must honor context. Synchronous filesystem lookups cannot be forcibly
// canceled; never abandon an I/O goroutine. Check expiration before classifying
// even an adapter-translated Unavailable as discovery eligible.
func boundedPreflight[T any](ctx context.Context, call func(context.Context) (T, error)) (T, error) {
	phase, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var zero T
	if err := phase.Err(); err != nil {
		return zero, err
	}
	value, err := call(phase)
	if expired := phase.Err(); expired != nil {
		return value, expired
	}
	return value, err
}

func (l *coexistenceLifecycle) resolveSourceBounded(ctx context.Context, host string) (*pb.LifecycleIdentity, string, error) {
	type resolved struct {
		identity *pb.LifecycleIdentity
		endpoint string
	}
	r, err := boundedPreflight(ctx, func(phase context.Context) (resolved, error) {
		id, endpoint, err := l.options.ResolveSource(phase, host)
		return resolved{id, endpoint}, err
	})
	return r.identity, r.endpoint, err
}
func (l *coexistenceLifecycle) recoverySelectionBounded(ctx context.Context, host string) (*pb.LifecycleRecoverySelection, error) {
	return boundedPreflight(ctx, func(phase context.Context) (*pb.LifecycleRecoverySelection, error) {
		return l.options.RecoverySelection(phase, host)
	})
}
