package server

// These controls are disabled by default. Loopback is NOT authentication:
// startup must provide ControlAuthorize and a ControlDial that authenticates the
// selected peer. No transport disconnect cancels an accepted incoming handover.
import (
	"context"
	"errors"
	"fmt"
	"net"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func controlLoopback(endpoint string) bool {
	h, _, e := net.SplitHostPort(endpoint)
	ip := net.ParseIP(h)
	return e == nil && ip != nil && ip.IsLoopback()
}
func (l *coexistenceLifecycle) controlIdentity() *pb.LifecycleIdentity {
	return &pb.LifecycleIdentity{Host: l.options.Host, Build: l.options.Build, Contract: l.options.Contract.Contract, Schema: int32(l.options.Contract.SchemaVersion), Store: l.options.StorePath}
}
func wireOwner(o broker.Owner) *pb.LifecycleOwner {
	return &pb.LifecycleOwner{Conversation: o.Conversation, Host: o.Host, Epoch: o.Epoch}
}
func controlOwner(r *pb.LifecycleControlRequest) broker.Owner {
	return broker.Owner{Conversation: r.GetOwner().GetConversation(), Host: r.GetOwner().GetHost(), Epoch: r.GetOwner().GetEpoch()}
}
func (s *Server) control(ctx context.Context, r *pb.LifecycleControlRequest) (*coexistenceLifecycle, error) {
	l, e := s.lifecycle()
	if e != nil {
		return nil, e
	}
	if l.options.ControlAuthorize == nil {
		return nil, status.Error(codes.PermissionDenied, "lifecycle authentication trust not configured")
	}
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil || !controlLoopback(p.Addr.String()) {
		return nil, status.Error(codes.PermissionDenied, "loopback control peer required")
	}
	if e = l.options.ControlAuthorize(ctx); e != nil {
		return nil, e
	}
	if r == nil || !proto.Equal(r.Target, l.controlIdentity()) {
		return nil, status.Error(codes.FailedPrecondition, "exact selected identity required")
	}
	return l, nil
}
func (s *Server) LifecycleAcquire(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	o, e := s.Retain(ctx, r.GetOwner().GetConversation())
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: wireOwner(o)}, e
}
func (s *Server) LifecyclePrepare(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	src := r.Source
	if src == nil || src.Host != r.GetOwner().GetHost() || src.Store != l.options.StorePath || src.Contract != l.options.Contract.Contract || src.Schema != int32(l.options.Contract.SchemaVersion) || !proto.Equal(r.Destination, r.Target) || !controlLoopback(r.DestinationEndpoint) {
		return nil, fmt.Errorf("incompatible selected source/destination")
	}
	a, e := s.AcceptIncoming(controlOwner(r), r.HandoverId, src.Build, conversation.CoexistenceDeclaration{Contract: src.Contract, SchemaVersion: int(src.Schema)})
	if e != nil {
		return nil, e
	}
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Receipt: a.receipt, Pending: true}, nil
}
func (l *coexistenceLifecycle) receiptLocked(r *pb.LifecycleControlRequest) *IncomingAcceptance {
	for _, a := range l.incoming {
		if a.receipt == r.Receipt && r.Receipt != "" && a.sourceBuild == r.GetSource().GetBuild() && a.h == (broker.IncomingHandover{Conversation: r.GetOwner().GetConversation(), SourceHost: r.GetOwner().GetHost(), SourceEpoch: r.GetOwner().GetEpoch(), HandoverID: r.HandoverId, DestinationHost: l.options.Host}) && r.GetSource().GetHost() == a.h.SourceHost && r.GetSource().GetStore() == l.options.StorePath && r.GetSource().GetContract() == l.options.Contract.Contract && r.GetSource().GetSchema() == int32(l.options.Contract.SchemaVersion) {
			return a
		}
	}
	return nil
}
func (s *Server) LifecycleValidateReceipt(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.receiptLocked(r) == nil {
		return nil, broker.ErrAdmissionStale
	}
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Receipt: r.Receipt, Pending: true}, nil
}
func (s *Server) LifecycleClaim(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	l.mu.Lock()
	a := l.receiptLocked(r)
	l.mu.Unlock()
	if a == nil {
		return nil, broker.ErrAdmissionStale
	}
	o, e := s.ClaimIncoming(ctx, a)
	var claimed *conversation.HandoverAlreadyClaimed
	if errors.As(e, &claimed) {
		o = broker.Owner{Conversation: a.h.Conversation, Host: l.options.Host, Epoch: claimed.Epoch}
		e = nil
	}
	if e != nil {
		return nil, e
	}
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: wireOwner(o)}, nil
}

// An uncertain peer response keeps the exact closed source reservation. Only
// an explicit retry of the same selection may reuse it, never a new transfer.
type outgoingControl struct {
	request    *pb.LifecycleControlRequest
	active     bool
	committing bool
	receipt    string
	completed  bool
}

func (s *Server) controlRelinquish(ctx context.Context, r *pb.LifecycleControlRequest, transfer bool) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	o := controlOwner(r)
	terminal := ""
	if !r.Management {
		bound, id, err := ParseTerminalBinding(ctx, o.Conversation)
		if err != nil {
			return nil, err
		}
		if bound != o {
			return nil, broker.ErrAdmissionStale
		}
		terminal = id
	}
	var h *broker.IncomingHandover
	var reserved *relinquishReservation
	receipt := r.Receipt
	if transfer {
		if r.HandoverId == "" || l.options.ControlDial == nil || !controlLoopback(r.DestinationEndpoint) || !proto.Equal(r.Source, l.controlIdentity()) || r.Destination == nil || r.Destination.Host == l.options.Host || r.Destination.Store != l.options.StorePath || r.Destination.Contract != l.options.Contract.Contract || r.Destination.Schema != int32(l.options.Contract.SchemaVersion) {
			return nil, fmt.Errorf("authenticated explicit compatible destination required")
		}
		// Close/check quiescence and retain persistence participation before ANY
		// peer RPC. Never hold the lifecycle mutex across a cross-host call.
		l.mu.Lock()
		key := proto.Clone(r).(*pb.LifecycleControlRequest)
		key.Receipt = ""
		reserved = l.closures[o]
		if reserved != nil {
			state := reserved.control
			if state != nil && !state.active && state.committing && proto.Equal(state.request, key) && (receipt == "" || receipt == state.receipt) {
				response := &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Receipt: state.receipt, Pending: true}
				l.mu.Unlock()
				if state.completed {
					return response, nil
				}
				st, _ := status.New(codes.FailedPrecondition, "source commit outcome unresolved; explicit claim may be retried").WithDetails(response)
				return nil, st.Err()
			}
			if state == nil || state.active || state.committing || !proto.Equal(state.request, key) {
				e = broker.ErrAdmissionStale
			} else {
				state.active = true
			}
		} else {
			reserved, e = l.reserveRelinquishLocked(o, nil, &terminal)
			if e == nil {
				reserved.control = &outgoingControl{request: key, active: true}
			}
		}
		l.mu.Unlock()
		if e != nil {
			return nil, e
		}
		defer func() { l.mu.Lock(); reserved.control.active = false; l.mu.Unlock() }()
		conn, err := l.options.ControlDial(ctx, r.DestinationEndpoint)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		check := proto.Clone(r).(*pb.LifecycleControlRequest)
		check.Target = check.Destination
		var proof *pb.LifecycleControlResponse
		client := pb.NewAgentClient(conn)
		if receipt == "" {
			proof, err = client.LifecyclePrepare(ctx, check)
		} else {
			proof, err = client.LifecycleValidateReceipt(ctx, check)
		}
		if err != nil {
			return nil, err
		}
		if !proto.Equal(proof.Identity, r.Destination) || proof.Receipt == "" || (receipt != "" && proof.Receipt != receipt) {
			return nil, broker.ErrAdmissionStale
		}
		receipt = proof.Receipt
		h = &broker.IncomingHandover{Conversation: o.Conversation, SourceHost: o.Host, SourceEpoch: o.Epoch, HandoverID: r.HandoverId, DestinationHost: r.Destination.Host}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if transfer {
		reserved.control.committing = true
		reserved.control.receipt = receipt
		e = l.commitRelinquishLocked(ctx, o, reserved, h)
		reserved.control.completed = e == nil
	} else {
		e = l.relinquishLocked(ctx, o, nil, &terminal, h)
	}
	response := &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Receipt: receipt, Pending: transfer}
	if e != nil {
		// gRPC discards response messages on error; preserve known receipts in
		// status details for explicit claim retries, never report early success.
		st, _ := status.New(codes.FailedPrecondition, e.Error()).WithDetails(response)
		return nil, st.Err()
	}
	return response, nil
}
func (s *Server) LifecycleRelease(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	return s.controlRelinquish(ctx, r, false)
}
func (s *Server) LifecycleRelinquish(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	return s.controlRelinquish(ctx, r, true)
}

// Ready is deliberately conservative: this listing does not reserve retirement.
func (s *Server) LifecycleList(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := &pb.LifecycleControlResponse{Identity: l.controlIdentity()}
	for _, cap := range l.owners {
		o := ownerOf(cap)
		out.Retained = append(out.Retained, wireOwner(o))
		_, retained, open := l.admission.CheckRetained(o)
		if retained && open {
			out.OpenOwners = append(out.OpenOwners, wireOwner(o))
		}
	}
	for _, a := range l.incoming {
		if _, err := conversation.HandoverResult(ctx, l.store, a.h.Conversation, l.options.Host, a.h.HandoverID); err != nil {
			out.PendingIncoming++
		}
	}
	return out, nil
}
func (s *Server) LifecycleRetire(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	e = s.RetireCoexistence(ctx)
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Ready: e == nil}, e
}
