package persistence

import (
	"context"
	"sync"
	"time"
)

type outcomeKey struct{}
type writeOutcome struct {
	mu     sync.Mutex
	err    error
	failed func(error)
}

// WithWriteFailure records the first failed durable write and notifies the
// admission owner. Install before execution; descendants share the outcome.
// The callback must not re-enter this outcome. There is no reconciliation API.
func WithWriteFailure(ctx context.Context, failed func(error)) context.Context {
	return context.WithValue(ctx, outcomeKey{}, &writeOutcome{failed: failed})
}

// ReportWriteFailure makes legacy void persistence callbacks observable to
// their owner. Callers must suppress dependent success and wake publication.
func ReportWriteFailure(ctx context.Context, err error) {
	if err == nil {
		return
	}
	o, _ := ctx.Value(outcomeKey{}).(*writeOutcome)
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.err != nil {
		return
	}
	o.err = err
	if o.failed != nil {
		o.failed(err)
	}
}

func WriteFailure(ctx context.Context) error {
	o, _ := ctx.Value(outcomeKey{}).(*writeOutcome)
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.err
}

// BoundedWriteContext preserves ownership and failure accounting after a
// transport disconnect. The caller must retain its work ticket until cancel
// AND the synchronous write/callback have finished, not merely until timeout.
func BoundedWriteContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}
