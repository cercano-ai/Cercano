package server

import (
	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/recap"
	"context"
)

// PreparedSchedulingFactory creates host-owned producers on the prepared store.
// It must not start legacy schedules. Nil producers leave that producer disabled.
// It is only invoked by explicit prepared construction, never canonical startup.
type PreparedSchedulingFactory func(conversation.Store) (*recap.Generator, *compactiongen.Generator)

type preparedScheduling struct {
	recap      *recap.Generator
	compaction *compactiongen.Generator
}

func (s *Server) installPreparedScheduling(a *agent.Agent, factory PreparedSchedulingFactory) {
	r, c := factory(s.persistSvc.Store())
	s.preparedScheduling = &preparedScheduling{r, c}
	agent.WithOwnedScheduling(&agent.OwnedScheduling{
		ReportFailure: persistsvc.ReportWriteFailure,
		Recap: func(ctx context.Context, id string) error {
			cap, err := s.resolvePreparedTaskOwner(ctx, id)
			if err != nil {
				return err
			}
			owner := ownerOf(cap)
			if s.workFenced.Load() {
				return broker.ErrAdmissionStale
			}
			if r == nil {
				return nil
			}
			_, err = persistsvc.ScheduleOwnedRecap(ctx, r, owner, s.BeginRetainedWork)
			return err
		},
		Compaction: func(ctx context.Context, id string) error {
			cap, err := s.resolvePreparedTaskOwner(ctx, id)
			if err != nil {
				return err
			}
			owner := ownerOf(cap)
			if s.workFenced.Load() {
				return broker.ErrAdmissionStale
			}
			if c == nil {
				return nil
			}
			_, err = persistsvc.ScheduleOwnedCompaction(ctx, c, owner, s.BeginRetainedWork)
			return err
		},
	})(a)
}

func (p *preparedScheduling) fence() {
	if p == nil {
		return
	}
	if p.recap != nil {
		p.recap.Fence()
	}
	if p.compaction != nil {
		p.compaction.Fence()
	}
}
func (p *preparedScheduling) close() {
	if p == nil {
		return
	}
	p.fence()
	// Cancellation is not completion: retain shutdown ownership until all writes
	// and final callbacks really return, independently of any requesting RPC.
	if p.recap != nil {
		_ = p.recap.Close(context.Background())
	}
	if p.compaction != nil {
		_ = p.compaction.Close(context.Background())
	}
}
