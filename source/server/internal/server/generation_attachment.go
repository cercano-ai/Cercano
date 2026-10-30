package server

import (
	"context"
	"fmt"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/worker"
)

// AttachGenerationLifecycle transfers an unopened-to-requests lifecycle onto an
// ordinary service graph. Call only during startup, before admitting any work.
// Configuration, credentials, tools, and watchdog remain those of the ordinary
// constructor. The caller must have composed the agent on the exact lifecycle
// store; attachment never opens, adopts, or substitutes a database.
func (s *Server) AttachGenerationLifecycle(lifecycle *Server, scheduling PreparedSchedulingFactory) error {
	if s == nil || lifecycle == nil || lifecycle == s || lifecycle.coexistence == nil || scheduling == nil {
		return fmt.Errorf("generation lifecycle and scheduling required")
	}
	if s.coexistence != nil || s.workerRunner != nil || s.preparedScheduling != nil {
		return fmt.Errorf("generation attachment must precede execution configuration")
	}
	l := lifecycle.coexistence
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.owners) != 0 {
		return fmt.Errorf("generation attachment requires an unused lifecycle")
	}
	if s.agent == nil || s.persistSvc == nil || s.agent.PersistentStore() != l.store || s.persistSvc.Store() != l.store {
		return fmt.Errorf("ordinary services must use the exact generation lifecycle store")
	}
	s.coexistence = l
	s.ordinaryGeneration = true
	l.options.Reload = nil
	l.options.StageReload = func(ctx context.Context, store conversation.Store, cap *conversation.Ownership) (func(context.Context) error, error) {
		return s.stagePreparedReload(ctx, store, cap)
	}
	s.installPreparedScheduling(s.agent, scheduling)
	lifecycle.coexistence = nil
	return nil
}

// ConfigureGenerationWorker selects the immutable generation binary after
// lifecycle attachment, using the already-composed ordinary host services.
func (s *Server) ConfigureGenerationWorker(policy worker.SpawnPolicy) error {
	return s.configureLifecycleWorker(policy)
}
