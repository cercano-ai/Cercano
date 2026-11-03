package server

import (
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"context"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LifecycleRecover runs on the destination. Inspection does not change durable
// state; confirmed recovery repeats death verification and exact epoch checks.
func (s *Server) LifecycleRecover(ctx context.Context, r *pb.LifecycleRecoverRequest) (*pb.LifecycleControlResponse, error) {
	l, err := s.control(ctx, &pb.LifecycleControlRequest{Target: r.GetTarget(), Management: true})
	if err != nil {
		return nil, err
	}
	if r.GetConversation() == "" {
		return nil, status.Error(codes.InvalidArgument, "conversation required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.options.RecoveryGuard == nil {
		return nil, status.Error(codes.FailedPrecondition, "verified dead-owner recovery unavailable")
	}
	v, err := conversation.ReadOwnership(ctx, l.store, r.Conversation)
	if err != nil {
		return nil, err
	}
	expected := r.GetExpectedOwner()
	if r.Confirm && (expected == nil || expected.Conversation != r.Conversation || expected.Host == "" || expected.Epoch == "") {
		return nil, status.Error(codes.InvalidArgument, "exact expected recovery owner required")
	}
	if r.Confirm && expected != nil && v.Owner == l.options.Host {
		before := conversation.OwnershipState{Owner: expected.Host, Epoch: expected.Epoch, State: "owned"}
		epoch, e := conversation.HandoverResult(ctx, l.store, r.Conversation, l.options.Host, conversation.RecoveryID(r.Conversation, before, l.options.Host))
		cap := l.owners[r.Conversation]
		if e == nil && cap != nil && epoch == v.Epoch && l.ready[ownerOf(cap)] {
			return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: r.Conversation, Host: v.Owner, Epoch: v.Epoch}, Ready: true}, nil
		}
		return nil, status.Error(codes.FailedPrecondition, "recovery outcome not ready; no new epoch acquired")
	}
	if v.State != "owned" || v.Owner == l.options.Host {
		return nil, status.Error(codes.FailedPrecondition, "not an abandoned foreign owner")
	}
	if r.Confirm && (expected == nil || expected.Conversation != r.Conversation || expected.Host != v.Owner || expected.Epoch != v.Epoch) {
		return nil, status.Error(codes.FailedPrecondition, "stale recovery confirmation")
	}
	var release func()
	defer func() {
		if release != nil {
			release()
		}
	}()
	verify := func(ctx context.Context) error {
		var e error
		release, e = l.options.RecoveryGuard(ctx, v.Owner)
		if e != nil {
			return status.Errorf(codes.FailedPrecondition, "source %q death not verified; safe live-owner fencing unavailable (no source RPC attempted): %v", v.Owner, e)
		}
		if release == nil {
			return fmt.Errorf("recovery guard omitted exclusion")
		}
		return nil
	}
	if !r.Confirm {
		if err = verify(ctx); err != nil {
			return nil, err
		}
		return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: r.Conversation, Host: v.Owner, Epoch: v.Epoch}}, nil
	}
	owner, err := l.acquireUsing(ctx, r.Conversation, nil, nil, func() (*conversation.Ownership, error) {
		return conversation.RecoverDeadOwner(ctx, l.store, r.Conversation, v, l.options.Host, verify)
	})
	if err != nil {
		return nil, err
	}
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: owner.Conversation, Host: owner.Host, Epoch: owner.Epoch}, Ready: true}, nil
}
