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

// These are separate, authenticated protocol capabilities. Unimplemented is a
// refusal, never permission to fall back to the older handover/force controls.
func (s *Server) LifecycleTakeoverProbe(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	if r.GetOwner().GetHost() == l.options.Host {
		l.mu.Lock()
		defer l.mu.Unlock()
		cap := l.owners[r.Owner.Conversation]
		if cap == nil || ownerOf(cap) != controlOwner(r) || !l.ready[controlOwner(r)] {
			return nil, status.Error(codes.FailedPrecondition, "source no longer retains safe owner-epoch quiescence authority")
		}
	}
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity()}, nil
}
func (s *Server) LifecycleTakeoverPrepare(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	return s.LifecyclePrepare(ctx, r)
}
func (s *Server) LifecycleTakeoverReceipt(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	return s.LifecycleValidateReceipt(ctx, r)
}
func (s *Server) LifecycleTakeoverClaim(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	return s.LifecycleClaim(ctx, r) // includes ready capability + current durable epoch
}

// LifecycleTakeoverRelinquish quiesces an epoch rather than retrying an outgoing
// operation. An abandoned request can select a different destination, but cannot
// authorize continuing producers or release an uncertain persistence ticket.
func (s *Server) LifecycleTakeoverRelinquish(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	o := controlOwner(r)
	if !r.Management || !proto.Equal(r.Source, r.Target) || o.Host != l.options.Host || o.Epoch == "" || r.HandoverId == "" || r.Receipt == "" || r.Destination == nil || r.Destination.Host == o.Host || r.Destination.Store != l.options.StorePath || r.Destination.Contract != l.options.Contract.Contract || r.Destination.Schema != int32(l.options.Contract.SchemaVersion) || !controlLoopback(r.DestinationEndpoint) || l.options.ControlDial == nil {
		return nil, status.Error(codes.FailedPrecondition, "exact compatible takeover destination and acceptance required")
	}
	// Authenticate the destination and its NEW capability/acceptance before any
	// source change. This can race an old claim; the durable CAS must decide it.
	conn, e := l.options.ControlDial(ctx, r.DestinationEndpoint, r.Destination)
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	check := proto.Clone(r).(*pb.LifecycleControlRequest)
	check.Target = r.Destination
	proof, e := pb.NewAgentClient(conn).LifecycleTakeoverReceipt(ctx, check)
	if e != nil {
		return nil, e
	}
	if !proto.Equal(proof.GetIdentity(), r.Destination) || proof.GetReceipt() != r.Receipt {
		return nil, broker.ErrAdmissionStale
	}
	key := proto.Clone(r).(*pb.LifecycleControlRequest)
	key.DestinationEndpoint = ""
	l.mu.Lock()
	// Reuse only our exact result, not another request's result or uncertainty.
	old := l.closures[o]
	if old != nil && old.control != nil && old.control.takeover && proto.Equal(old.control.request, key) && old.control.completed {
		l.mu.Unlock()
		return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Receipt: r.Receipt, Pending: true}, nil
	}
	cap := l.owners[o.Conversation]
	if cap == nil || ownerOf(cap) != o || !l.ready[o] {
		l.mu.Unlock()
		return nil, status.Error(codes.FailedPrecondition, "source has no retained safe quiescence authority; committed old handover cannot be superseded here")
	}
	if old != nil && (old.control == nil || old.control.active || old.control.committing || (old.control.takeover && !proto.Equal(old.control.request, key))) {
		l.mu.Unlock()
		return nil, status.Error(codes.FailedPrecondition, "source control active or persistence outcome uncertain; source stays fenced")
	}
	closure, e := l.admission.QuiesceOwner(o)
	if e != nil {
		l.mu.Unlock()
		return nil, e
	}
	if old == nil {
		ticket, err := l.admission.AdmitHostWork(broker.PersistenceWork)
		if err != nil {
			l.mu.Unlock()
			return nil, err
		}
		old = &relinquishReservation{closure: closure, ticket: ticket}
		if l.closures == nil {
			l.closures = make(map[broker.Owner]*relinquishReservation)
		}
		l.closures[o] = old
	}
	old.control = &outgoingControl{request: key, active: true, takeover: true, receipt: r.Receipt}
	l.mu.Unlock()
	defer func() { l.mu.Lock(); old.control.active = false; l.mu.Unlock() }()
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
		pending, uncertain, err := closure.Pending()
		if err != nil {
			return nil, err
		}
		if uncertain != 0 {
			return nil, status.Error(codes.FailedPrecondition, "takeover fenced source; unresolved persistence outcomes must resolve before ownership changes")
		}
		if pending == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, fmt.Sprintf("takeover fenced source; %d completion tickets remain", pending))
		case <-ticker.C:
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	old.control.committing = true
	// ReserveTakeover is a transaction-locked exact-source CAS. In particular an
	// old destination claim winning the race must never be overwritten.
	if e = cap.ReserveTakeover(ctx, r.Destination.Host, r.HandoverId); e != nil {
		_ = old.ticket.PersistenceFailed()
		return nil, e
	}
	if e = closure.Relinquished(); e != nil {
		_ = old.ticket.PersistenceFailed()
		return nil, e
	}
	delete(l.owners, o.Conversation)
	delete(l.ready, o)
	if l.options.Cleanup != nil {
		if e = l.options.Cleanup(ctx, o); e != nil {
			_ = old.ticket.PersistenceFailed()
			return nil, e
		}
	}
	if e = old.ticket.Complete(); e != nil {
		return nil, e
	}
	old.control.completed = true
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Receipt: r.Receipt, Pending: true}, nil
}
