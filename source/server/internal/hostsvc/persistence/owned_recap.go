package persistence

import (
	"context"
	"errors"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/recap"
)

// ScheduleOwnedRecap is an opt-in bridge, not prepared scheduler installation.
// begin must validate the supplied exact owner and retain its store capability;
// it must never resolve current authority from a conversation ID.
func ScheduleOwnedRecap(ctx context.Context, g *recap.Generator, owner broker.Owner, begin recap.BeginOwnedWork) (*recap.ScheduledWork, error) {
	if g == nil || begin == nil {
		return nil, errors.New("owned recap requires generator and exact admission")
	}
	return g.ScheduleOwned(ctx, owner, begin)
}
