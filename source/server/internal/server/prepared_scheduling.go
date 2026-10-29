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
			// Serialize admission with dispatch finalization and retain the producer
			// result so child ownership cannot be released under delayed writeback.
			scope, _ := ctx.Value(preparedDispatchKey{}).(*preparedDispatchScope)
			if scope != nil {
				scope.mu.Lock()
				defer scope.mu.Unlock()
				if scope.closed {
					return broker.ErrAdmissionClosed
				}
			}
			work, err := persistsvc.ScheduleOwnedRecap(ctx, r, owner, s.BeginRetainedWork)
			if err == nil && scope != nil {
				scope.waits = append(scope.waits, func() error { <-work.Done; return work.Err })
			}
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
			// Serialize admission with dispatch finalization and retain the producer
			// result so child ownership cannot be released under delayed writeback.
			scope, _ := ctx.Value(preparedDispatchKey{}).(*preparedDispatchScope)
			if scope != nil {
				scope.mu.Lock()
				defer scope.mu.Unlock()
				if scope.closed {
					return broker.ErrAdmissionClosed
				}
			}
			work, err := persistsvc.ScheduleOwnedCompaction(ctx, c, owner, s.BeginRetainedWork)
			if err == nil && scope != nil {
				scope.waits = append(scope.waits, func() error { <-work.Done; return work.Err })
			}
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
