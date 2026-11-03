package server

import (
	"context"
	"fmt"
	"time"

	"cercano/source/server/internal/broker"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// LifecycleForceRelinquish is intentionally a distinct RPC. An old host must
// reject it rather than interpret force as the old terminal-only takeover.
func (s *Server) LifecycleForceRelinquish(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, err := s.control(ctx, r)
	if err != nil {
		return nil, err
	}
	o := controlOwner(r)
	if !r.Management || r.Destination == nil || r.Destination.Host == o.Host || r.HandoverId == "" || r.GetOwner().GetHost() != l.options.Host || !proto.Equal(r.Source, r.Target) || r.Destination.Store != l.options.StorePath || r.Destination.Contract != l.options.Contract.Contract || r.Destination.Schema != int32(l.options.Contract.SchemaVersion) || !controlLoopback(r.DestinationEndpoint) || l.options.ControlDial == nil {
		return nil, status.Error(codes.FailedPrecondition, "incompatible selected cutover source/destination")
	}
	// Authenticate the exact destination before revoking anything. This read-only
	// preflight grants no ownership and cannot publish a new default target.
	conn, err := l.options.ControlDial(ctx, r.DestinationEndpoint)
	if err != nil {
		return nil, err
	}
	check, err := pb.NewAgentClient(conn).LifecycleList(ctx, &pb.LifecycleControlRequest{Target: r.Destination})
	conn.Close()
	if err != nil {
		return nil, err
	}
	if !proto.Equal(check.GetIdentity(), r.Destination) {
		return nil, status.Error(codes.FailedPrecondition, "cutover destination identity changed")
	}

	key := proto.Clone(r).(*pb.LifecycleControlRequest)
	key.Receipt = ""
	key.DestinationEndpoint = "" // authenticated identity, not a transient route, binds retries
	l.mu.Lock()
	existing := l.closures[o]
	if existing != nil && (existing.control == nil || !proto.Equal(existing.control.request, key) || !l.forced[o]) {
		l.mu.Unlock()
		return nil, broker.ErrAdmissionBusy
	}
	if existing == nil {
		cap := l.owners[o.Conversation]
		if cap == nil || ownerOf(cap) != o || !l.ready[o] {
			l.mu.Unlock()
			return nil, broker.ErrAdmissionStale
		}
		ticket, e := l.admission.AdmitHostWork(broker.PersistenceWork)
		if e != nil {
			l.mu.Unlock()
			return nil, e
		}
		closure, e := l.admission.ForceCloseOwner(o)
		if e != nil {
			_ = ticket.Complete()
			l.mu.Unlock()
			return nil, e
		}
		existing = &relinquishReservation{closure: closure, ticket: ticket, control: &outgoingControl{request: key}}
		if l.closures == nil {
			l.closures = make(map[broker.Owner]*relinquishReservation)
		}
		if l.forced == nil {
			l.forced = make(map[broker.Owner]bool)
		}
		l.closures[o], l.forced[o] = existing, true
	}
	committing := existing.control.committing
	l.mu.Unlock()
	if committing {
		return s.LifecycleRelinquish(ctx, r)
	}

	// Transport references have already been revoked, regardless of whether a
	// turn exists. Cancel actual execution separately; never cancel sibling chats.
	if s.turnBroker != nil {
		s.turnBroker.CancelTurn(o.Conversation)
	}
	s.taskRunMu.Lock()
	manager := s.taskManager
	s.taskRunMu.Unlock()
	if manager != nil {
		manager.CancelConversation(o.Conversation, o.Epoch)
	}

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		pending, uncertain, e := existing.closure.Pending()
		if e != nil {
			return nil, e
		}
		if uncertain != 0 {
			return nil, status.Errorf(codes.FailedPrecondition, "forced cutover fenced old clients; %d unresolved persistence outcomes remain (not evidence of running work)", uncertain)
		}
		if pending == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, fmt.Sprintf("forced cutover cancelled old execution but %d completion tickets remain; source stays fenced; retry this cutover", pending))
		case <-ticker.C:
		}
	}
	// Existing authenticated prepare/reserve/claim machinery supplies the fresh
	// durable epoch. It cannot run until every old producer has finished writeback.
	return s.LifecycleRelinquish(ctx, r)
}

func (s *Server) forcedCutover(o broker.Owner) bool {
	if s.coexistence == nil {
		return false
	}
	s.coexistence.mu.Lock()
	defer s.coexistence.mu.Unlock()
	return s.coexistence.forced[o]
}
