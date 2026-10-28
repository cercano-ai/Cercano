package agent

import (
	"context"
)

// OwnedScheduling is opt-in, installed before a prepared agent is exposed.
// Callbacks must derive authority from the incoming capability, never an ID lookup.
// The scheduling callbacks admit the exact owner before creating any timer.
type OwnedScheduling struct {
	ReportFailure func(context.Context, error)
	Recap         func(context.Context, string) error
	Compaction    func(context.Context, string) error
}

func WithOwnedScheduling(s *OwnedScheduling) AgentOption {
	return func(a *Agent) { a.ownedScheduling = s }
}
func (a *Agent) HasOwnedScheduling() bool { return a.ownedScheduling != nil }
func (a *Agent) ScheduleRecapContext(ctx context.Context, id string) error {
	if s := a.ownedScheduling; s != nil {
		return s.Recap(ctx, id)
	}
	a.ScheduleRecap(id)
	return nil
}
func (a *Agent) ScheduleCompactionContext(ctx context.Context, id string) error {
	if s := a.ownedScheduling; s != nil {
		return s.Compaction(ctx, id)
	}
	a.ScheduleCompaction(id)
	return nil
}

// SchedulePersistedAssistant is deliberately a legacy no-op: legacy persistence
// callers already schedule after their turn and must not acquire new behavior.
func (a *Agent) SchedulePersistedAssistant(ctx context.Context, id string) error {
	if !a.HasOwnedScheduling() {
		return nil
	}
	if err := a.ScheduleRecapContext(ctx, id); err != nil {
		return err
	}
	return a.ScheduleCompactionContext(ctx, id)
}

func (a *Agent) reportOwnedFailure(ctx context.Context, err error) bool {
	if err == nil || a.ownedScheduling == nil {
		return false
	}
	if a.ownedScheduling.ReportFailure != nil {
		a.ownedScheduling.ReportFailure(ctx, err)
	}
	return true
}
