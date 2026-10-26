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
	o := l.owners[owner.Conversation]
	if o == nil || ownerOf(o) != owner {
		return nil, nil, broker.ErrAdmissionStale
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
