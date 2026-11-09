package server

import (
	"context"
	"errors"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// retrySelection is deliberately narrower than normal transfer controls. It
// cannot generate an operation, accept a destination, or acquire an owner.
func retrySelection(r *pb.LifecycleControlRequest) bool {
	return r != nil && r.Management && r.GetOwner().GetConversation() != "" && r.GetOwner().GetEpoch() != "" && r.GetOwner().GetHost() == r.GetSource().GetHost() && r.GetSource().GetHost() != "" && r.GetSource().GetBuild() != "" && r.GetDestination().GetHost() != "" && r.GetDestination().GetBuild() != "" && r.Source.Host != r.Destination.Host && r.HandoverId != "" && r.Receipt != ""
}
func retryStale() error {
	return status.Error(codes.FailedPrecondition, "exact existing transfer state required; retry only the original operation")
}

// LifecycleRetryReceipt is both a read-only acceptance check and the explicit
// retry-only capability boundary. An old server returns Unimplemented.
func (s *Server) LifecycleRetryReceipt(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	if !retrySelection(r) || !proto.Equal(r.Destination, r.Target) {
		return nil, retryStale()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.receiptLocked(r) == nil {
		return nil, retryStale()
	}
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Receipt: r.Receipt, Pending: true}, nil
}

// LifecycleRetryRelinquish never calls the normal source RPCs. In particular,
// there is no absent-reservation branch that can close/fence a new owner.
func (s *Server) LifecycleRetryRelinquish(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	if !retrySelection(r) || !proto.Equal(r.Source, r.Target) || !controlLoopback(r.DestinationEndpoint) || l.options.ControlDial == nil {
		return nil, retryStale()
	}
	o := controlOwner(r)
	key := proto.Clone(r).(*pb.LifecycleControlRequest)
	key.Receipt = ""
	key.DestinationEndpoint = ""
	matches := func(res *relinquishReservation) bool {
		return res != nil && res.control != nil && proto.Equal(res.control.request, key) && (res.control.receipt == "" || res.control.receipt == r.Receipt)
	}
	l.mu.Lock()
	res := l.closures[o]
	ok := matches(res)
	l.mu.Unlock()
	if !ok {
		return nil, retryStale()
	}
	// No lock across an authenticated peer call. This check is NOT a distributed
	// reservation: acceptance can disappear afterwards. The destination rechecks
	// locally at claim, while a source commit can remain durably reserved.
	conn, e := l.options.ControlDial(ctx, r.DestinationEndpoint, r.Destination)
	if e != nil {
		return nil, e
	}
	defer conn.Close()
	check := proto.Clone(r).(*pb.LifecycleControlRequest)
	check.Target = check.Destination
	proof, e := pb.NewAgentClient(conn).LifecycleRetryReceipt(ctx, check)
	if e != nil {
		return nil, e
	}
	if !proto.Equal(proof.GetIdentity(), r.Destination) || proof.GetReceipt() != r.Receipt {
		return nil, retryStale()
	}
	l.mu.Lock()
	if l.closures[o] != res || !matches(res) || res.control.active {
		l.mu.Unlock()
		return nil, retryStale()
	}
	state := res.control
	response := &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Receipt: r.Receipt, Pending: true}
	if state.committing {
		done := state.completed
		l.mu.Unlock()
		if done {
			return response, nil
		}
		return nil, status.Error(codes.FailedPrecondition, "source persistence outcome unresolved; no automatic retry of write")
	}
	cap := l.owners[o.Conversation]
	if cap == nil || ownerOf(cap) != o {
		l.mu.Unlock()
		return nil, retryStale()
	}
	state.active = true
	l.mu.Unlock()
	defer func() { l.mu.Lock(); state.active = false; l.mu.Unlock() }()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		pending, uncertain, err := res.closure.Pending()
		if err != nil {
			return nil, err
		}
		if uncertain != 0 {
			return nil, status.Error(codes.FailedPrecondition, "unresolved persistence outcomes remain; source stays fenced")
		}
		if pending == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, "transfer outcome uncertain; source stays fenced; retry only this exact operation")
		case <-ticker.C:
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closures[o] != res || !matches(res) {
		return nil, retryStale()
	}
	if e = ctx.Err(); e != nil {
		return nil, status.Error(codes.DeadlineExceeded, "transfer outcome uncertain; retry only this exact operation")
	}
	state.committing = true
	state.receipt = r.Receipt
	h := &broker.IncomingHandover{Conversation: o.Conversation, SourceHost: o.Host, SourceEpoch: o.Epoch, HandoverID: r.HandoverId, DestinationHost: r.Destination.Host}
	e = l.commitRelinquishLocked(ctx, o, res, h)
	state.completed = e == nil
	if e != nil {
		return nil, e
	}
	return response, nil
}

// LifecycleRetryClaim holds the existing lifecycle lock across acceptance,
// unresolved-local-acquisition checks and the store's transactional claim. It
// never accepts a transfer or invokes unqualified acquisition.
func (s *Server) LifecycleRetryClaim(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	if !retrySelection(r) || !proto.Equal(r.Destination, r.Target) {
		return nil, retryStale()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	accepted := l.receiptLocked(r)
	if accepted == nil {
		return nil, retryStale()
	}
	if cap := l.owners[r.Owner.Conversation]; cap != nil && !l.ready[ownerOf(cap)] {
		return nil, status.Error(codes.FailedPrecondition, "destination acquisition unresolved; retry cannot report completion")
	}
	var o broker.Owner
	if l.owners[r.Owner.Conversation] != nil {
		epoch, err := conversation.HandoverResult(ctx, l.store, r.Owner.Conversation, l.options.Host, r.HandoverId)
		if err != nil {
			return nil, err
		}
		o = broker.Owner{Conversation: r.Owner.Conversation, Host: l.options.Host, Epoch: epoch}
	} else {
		o, e = l.acquire(ctx, r.Owner.Conversation, accepted)
		var already *conversation.HandoverAlreadyClaimed
		if errors.As(e, &already) {
			o = broker.Owner{Conversation: r.Owner.Conversation, Host: l.options.Host, Epoch: already.Epoch}
		} else if e != nil {
			return nil, e
		}
	}
	v, e := conversation.ReadOwnership(ctx, l.store, r.Owner.Conversation)
	if e != nil {
		return nil, e
	}
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: wireOwner(o), Receipt: r.Receipt, CurrentOwner: v.State == "owned" && v.Owner == o.Host && v.Epoch == o.Epoch}, nil
}
