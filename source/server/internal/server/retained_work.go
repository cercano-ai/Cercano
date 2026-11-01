package server

import (
	"context"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BeginRetainedWork scopes a capability and a ticket to the EXACT retained
// identity. It never selects the current owner by conversation ID, acquires,
// resumes, or attaches. Hold the ticket across descendants and bounded writes;
// cancellation is not completion. Complete refuses after a reported write
// failure. This adapter alone does not enable ordinary server handlers.
func (s *Server) BeginRetainedWork(ctx context.Context, owner broker.Owner, kind broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
	l, err := s.lifecycle()
	if err != nil {
		return nil, nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return s.beginRetainedWorkLocked(ctx, owner, kind, l)
}

// Caller holds the lifecycle lock, including terminal enrollment/revocation.
func (s *Server) beginRetainedWorkLocked(ctx context.Context, owner broker.Owner, kind broker.WorkKind, l *coexistenceLifecycle) (context.Context, *broker.WorkTicket, error) {
	o := l.owners[owner.Conversation]
	if o == nil || ownerOf(o) != owner {
		return nil, nil, broker.ErrAdmissionStale
	}
	if !l.ready[owner] {
		return nil, nil, broker.ErrAdmissionBusy
	}
	ticket, err := l.admission.AdmitWork(owner, kind)
	if err != nil {
		return nil, nil, err
	}
	ctx = conversation.WithOwnership(ctx, o)
	ctx = WithPreparedTaskOwnership(ctx, owner, o)
	ctx = persistsvc.WithWriteFailure(ctx, func(error) {
		// Accounting changes kind without releasing the live work blocker.
		_ = ticket.Advance(broker.PersistenceWork)
		_ = ticket.PersistenceFailed()
	})
	return ctx, ticket, nil
}

// Until every mutation/scheduler has a ticket bridge, lifecycle-only instances
// must fail before persistDevMode, broker supersession, or completion claims.
func (s *Server) refuseUnintegratedCoexistenceWork() error {
	if s.coexistence != nil {
		return status.Error(codes.FailedPrecondition, "coexistence ordinary work is not enabled: persistence and scheduling admission bridges incomplete")
	}
	return nil
}

// Ordinary service operations are not prepared-service operations. Keep the
// prepared deny gate, but admit the ordinary operation against its exact owner
// before calling normal persistence/runtime services. Empty target means a
// shared operation: use the terminal binding, never a guessed current owner.
func (s *Server) beginOrdinaryServiceWork(ctx context.Context, target string) (context.Context, *foregroundWork, error) {
	if !s.ordinaryGeneration {
		if err := s.refuseUnintegratedCoexistenceWork(); err != nil {
			return nil, nil, err
		}
		return ctx, nil, nil
	}
	if target != "" {
		return s.beginForeground(ctx, target)
	}
	return s.beginSettingsWork(ctx)
}
