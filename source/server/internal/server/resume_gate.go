package server

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// observationLimit is lifecycle-wide, not per request. A timed-out observer keeps
// its slot until it actually returns; saturation never creates another goroutine.
const observationLimit = 8

type observationBudgetKey struct{}
type recoveryObservationKey struct{}

func recoveryObservationContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, recoveryObservationKey{}, true)
}

// Only read-only callbacks belong here. Coordinators, journal work, Prepare,
// ownership and stop operations must remain joined to their foreground caller.
// An unbuffered handoff assigns cleanup to exactly one owner.
func observePreflight[T any](ctx context.Context, l *coexistenceLifecycle, call func(context.Context) (T, error), discard func(T)) (T, error) {
	phase, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var zero T
	if err := phase.Err(); err != nil {
		return zero, err
	}
	var counter *int
	if l != nil {
		counter = &l.observations
		if recovery, _ := ctx.Value(recoveryObservationKey{}).(bool); recovery {
			counter = &l.recoveryObservations
		}
		l.observationMu.Lock()
		if *counter >= observationLimit {
			l.observationMu.Unlock()
			return zero, context.DeadlineExceeded
		}
		*counter++
		l.observationMu.Unlock()
	}
	type result struct {
		value T
		err   error
	}
	done := make(chan result)
	go func() {
		if l != nil {
			defer func() { l.observationMu.Lock(); *counter--; l.observationMu.Unlock() }()
		}
		value, err := call(phase)
		select {
		case done <- result{value, err}:
		case <-phase.Done():
			if discard != nil {
				discard(value)
			}
		}
	}()
	select {
	case <-phase.Done():
		return zero, phase.Err()
	case r := <-done:
		if err := phase.Err(); err != nil {
			if discard != nil {
				discard(r.value)
			}
			return zero, err
		}
		return r.value, r.err
	}
}

func observationContext(ctx context.Context) context.Context {
	if budget, ok := ctx.Value(observationBudgetKey{}).(context.Context); ok {
		return budget
	}
	return ctx
}

func (l *coexistenceLifecycle) resolveSourceBounded(ctx context.Context, host string) (*pb.LifecycleIdentity, string, error) {
	type resolved struct {
		identity *pb.LifecycleIdentity
		endpoint string
	}
	r, err := observePreflight(observationContext(ctx), l, func(phase context.Context) (resolved, error) {
		id, endpoint, err := l.options.ResolveSource(phase, host)
		return resolved{id, endpoint}, err
	}, nil)
	return r.identity, r.endpoint, err
}
func (l *coexistenceLifecycle) recoverySelectionBounded(ctx context.Context, host string) (*pb.LifecycleRecoverySelection, error) {
	return observePreflight(recoveryObservationContext(observationContext(ctx)), l, func(phase context.Context) (*pb.LifecycleRecoverySelection, error) {
		return l.options.RecoverySelection(phase, host)
	}, nil)
}

// Recovery uses an independent trusted-record observer, never normal discovery.
func (l *coexistenceLifecycle) recoverySourceBounded(ctx context.Context, host string) (*pb.LifecycleIdentity, string, error) {
	type resolved struct {
		identity *pb.LifecycleIdentity
		endpoint string
	}
	resolver := l.options.RecoveryResolveSource
	if resolver == nil {
		return nil, "", status.Error(codes.FailedPrecondition, "independent recovery source observer unavailable")
	}
	r, err := observePreflight(recoveryObservationContext(ctx), l, func(phase context.Context) (resolved, error) {
		id, endpoint, err := resolver(phase, host)
		return resolved{id, endpoint}, err
	}, nil)
	return r.identity, r.endpoint, err
}
