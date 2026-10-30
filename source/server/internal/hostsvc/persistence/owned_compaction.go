package persistence

import (
	"context"
	"errors"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compactiongen"
)

// ScheduleOwnedCompaction is the opt-in producer bridge. The caller supplies
// exact retained admission (for example BeginRetainedWork), not an ID resolver.
// It does not install or enable generators in prepared composition. Existing
// SetCompactionGenerator freeze policy remains unchanged.
func ScheduleOwnedCompaction(ctx context.Context, g *compactiongen.Generator, owner broker.Owner, begin compactiongen.BeginOwnedWork) (*compactiongen.ScheduledWork, error) {
	if g == nil || begin == nil {
		return nil, errors.New("owned compaction requires generator and exact admission")
	}
	return g.ScheduleOwned(ctx, owner, func(ctx context.Context, owner broker.Owner, kind broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
		ctx, ticket, err := begin(ctx, owner, kind)
		if err != nil {
			return nil, nil, err
		}
		parent := ctx
		ctx = WithWriteFailure(ctx, func(err error) {
			ReportWriteFailure(parent, err)
			_ = ticket.Advance(broker.PersistenceWork)
			_ = ticket.PersistenceFailed()
		})
		observed := ctx
		ctx = compactiongen.WithPersistenceOutcome(ctx, func() error { return WriteFailure(observed) })
		return ctx, ticket, nil
	})
}
