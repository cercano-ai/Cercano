package recap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"cercano/source/server/internal/broker"
)

// BeginOwnedWork must validate the exact identity and return its store-bound
// capability context. It must not acquire or select an owner by conversation ID.
type BeginOwnedWork func(context.Context, broker.Owner, broker.WorkKind) (context.Context, *broker.WorkTicket, error)

// ScheduledWork exposes completion and the retained ticket for explicit durable
// reconciliation after failure. Cancellation never resolves persistence failure.
type ScheduledWork struct {
	Ticket  *broker.WorkTicket
	Done    chan struct{}
	Err     error // fatal completion error; read only after Done closes
	Warning error // auxiliary failure, retried on the next schedule; read after Done
}

type persistenceOutcomeKey struct{}

// WithPersistenceOutcome adapts legacy void persistence callbacks to owned work.
// The observer is scoped to this admission; legacy Schedule ignores it.
func WithPersistenceOutcome(ctx context.Context, outcome func() error) context.Context {
	return context.WithValue(ctx, persistenceOutcomeKey{}, outcome)
}

func ownedPersistenceOutcome(ctx context.Context) error {
	if outcome, ok := ctx.Value(persistenceOutcomeKey{}).(func() error); ok {
		return outcome()
	}
	return nil
}

// ScheduleOwned is an opt-in counterpart to Schedule. Admission precedes the
// timer; one ticket spans the delay, execution and both durable writes.
// Recap retries only on the next explicit schedule, as in the legacy path.
// Unlike legacy debouncing, separate admissions are not coalesced: cancellation
// and authority from different callers must never replace each other.
func (g *Generator) ScheduleOwned(parent context.Context, owner broker.Owner, begin BeginOwnedWork) (*ScheduledWork, error) {
	if begin == nil || owner.Conversation == "" {
		return nil, errors.New("owned recap requires exact admission")
	}
	ctx, release, ok := g.startWork(parent)
	if !ok {
		return nil, context.Canceled
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
				work.Err = fmt.Errorf("recap panic: %v", r)
				_ = ticket.Advance(broker.PersistenceWork)
				_ = ticket.PersistenceFailed()
			}
			if err := ticket.Complete(); err != nil {
				work.Err = errors.Join(work.Err, err)
			}
		}()
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
		if g.fenced.Load() {
			work.Err = context.Canceled
			return
		}
		if err := ticket.Advance(broker.PersistenceWork); err != nil {
			work.Err = err
			return
		}
		// PersistenceWork conservatively covers inference as well: no authority gap
		// exists before either the store write or void usage callback can run.
		err := g.runRecap(ctx, owner.Conversation, true)
		if err != nil {
			var auxiliary *auxiliaryInferenceFailure
			if errors.As(err, &auxiliary) {
				work.Warning = err
				fmt.Fprintf(os.Stderr, "[recap] auxiliary inference failed; retry on next schedule for %s: %v\n", owner.Conversation, err)
				return // inference terminated; all persistence outcomes are known
			}
			work.Err = err
			// An error may follow a partially successful write. Keep the blocker until
			// an explicit reconciler establishes the durable outcome; never replay it.
			_ = ticket.PersistenceFailed()
			return
		}
		return
	}()
	return work, nil
}
