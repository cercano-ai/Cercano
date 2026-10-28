package compactiongen

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cercano/source/server/internal/broker"
)

// BeginOwnedWork must validate the exact identity and return its store-bound
// capability context. It must not acquire or select an owner by conversation ID.
type BeginOwnedWork func(context.Context, broker.Owner, broker.WorkKind) (context.Context, *broker.WorkTicket, error)

// ScheduledWork exposes completion and the retained ticket for explicit durable
// reconciliation after failure. Cancellation never resolves persistence failure.
type ScheduledWork struct {
	Ticket *broker.WorkTicket
	Done   chan struct{}
	Err    error // read only after Done closes
}

type ownedPass struct{ retry bool }
type ownedPassKey struct{}
type persistenceOutcomeKey struct{}

// WithPersistenceOutcome adapts legacy void write callbacks to owned execution.
func WithPersistenceOutcome(ctx context.Context, outcome func() error) context.Context {
	return context.WithValue(ctx, persistenceOutcomeKey{}, outcome)
}

func (g *Generator) reschedule(ctx context.Context, id string) {
	if p, ok := ctx.Value(ownedPassKey{}).(*ownedPass); ok {
		p.retry = true
		return
	}
	g.Schedule(id)
}

// ScheduleOwned is an opt-in counterpart to Schedule. Admission precedes the
// timer; one ticket spans every busy/backlog retry, execution and callback.
// Unlike legacy debouncing, separate admissions are not coalesced: cancellation
// and authority from different callers must never replace each other.
func (g *Generator) ScheduleOwned(parent context.Context, owner broker.Owner, begin BeginOwnedWork) (*ScheduledWork, error) {
	if begin == nil || owner.Conversation == "" {
		return nil, errors.New("owned compaction requires exact admission")
	}
	ctx, release, ok := g.startWork(parent)
	if !ok {
		return nil, context.Canceled
	}
	g.mu.Lock()
	enabled := g.enabled
	g.mu.Unlock()
	if !enabled {
		release()
		return nil, errors.New("compaction disabled")
	}
	ctx, ticket, err := begin(ctx, owner, broker.CallbackWork)
	if err != nil {
		release()
		return nil, err
	}
	if ctx == nil || ticket == nil {
		release()
		return nil, errors.New("invalid owned admission result")
	}
	work := &ScheduledWork{Ticket: ticket, Done: make(chan struct{})}
	go func() {
		defer release()
		defer close(work.Done)
		defer func() {
			if r := recover(); r != nil {
				work.Err = fmt.Errorf("compaction panic: %v", r)
				_ = ticket.Advance(broker.PersistenceWork)
				_ = ticket.PersistenceFailed()
			}
			if err := ticket.Complete(); err != nil {
				work.Err = errors.Join(work.Err, err)
			}
		}()
		for {
			timer := time.NewTimer(g.debounce)
			select {
			case <-ctx.Done():
				timer.Stop()
				work.Err = ctx.Err()
				return
			case <-timer.C:
			}
			if err := ctx.Err(); err != nil {
				work.Err = err
				return
			}
			g.mu.Lock()
			enabled := g.enabled
			g.mu.Unlock()
			if !enabled {
				return
			}
			if err := ticket.Advance(broker.PersistenceWork); err != nil {
				work.Err = err
				return
			}
			// PersistenceWork conservatively covers inference as well: no authority gap
			// exists before either the store write or void usage callback can run.
			pass := &ownedPass{}
			err := g.runCompaction(context.WithValue(ctx, ownedPassKey{}, pass), owner.Conversation)
			if outcome, ok := ctx.Value(persistenceOutcomeKey{}).(func() error); ok {
				err = errors.Join(err, outcome())
			}
			if err != nil {
				work.Err = err
				// An error may follow a partially successful write. Keep the blocker until
				// an explicit reconciler establishes the durable outcome; never replay it.
				_ = ticket.PersistenceFailed()
				return
			}
			if !pass.retry {
				return
			}
			if err := ticket.Advance(broker.CallbackWork); err != nil {
				work.Err = err
				return
			}
		}
	}()
	return work, nil
}

func ownedPersistenceOutcome(ctx context.Context) error {
	if outcome, ok := ctx.Value(persistenceOutcomeKey{}).(func() error); ok {
		return outcome()
	}
	return nil
}
