package server

import (
	"context"
	"database/sql"
	"errors"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetDevelopmentHostIdentity reports immutable selected build/store declarations.
// It never opens a store, acquires ownership, attaches, or touches credentials.
func (s *Server) GetDevelopmentHostIdentity(ctx context.Context, _ *proto.GetDevelopmentHostIdentityRequest) (*proto.GetDevelopmentHostIdentityResponse, error) {
	l, err := s.lifecycle()
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	retired, _, _ := l.admission.CheckRetained(broker.Owner{})
	return &proto.GetDevelopmentHostIdentityResponse{HostInstance: l.options.Host, Build: l.options.Build, Contract: l.options.Contract.Contract, SchemaVersion: int32(l.options.Contract.SchemaVersion), Retired: retired}, nil
}

// CheckRetainedSession is a point-in-time check, NOT an attachment or permission
// to work. Normal handlers must separately validate metadata and acquire exact
// retained-owner tickets; a release may follow immediately after this response.
func (s *Server) CheckRetainedSession(ctx context.Context, req *proto.CheckRetainedSessionRequest) (*proto.CheckRetainedSessionResponse, error) {
	if req.GetHostInstance() == "" || req.GetConversationId() == "" || req.GetOwnershipEpoch() == "" {
		return nil, status.Error(codes.InvalidArgument, "exact host, conversation and epoch required")
	}
	l, err := s.lifecycle()
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	owner := broker.Owner{Host: req.HostInstance, Conversation: req.ConversationId, Epoch: req.OwnershipEpoch}
	retired, retained, open := l.admission.CheckRetained(owner)
	result := func(state proto.RetainedSessionState) (*proto.CheckRetainedSessionResponse, error) {
		return &proto.CheckRetainedSessionResponse{State: state}, nil
	}
	if owner.Host != l.options.Host {
		return result(proto.RetainedSessionState_RETAINED_SESSION_STATE_STALE)
	}
	if retired {
		return result(proto.RetainedSessionState_RETAINED_SESSION_STATE_RETIRED)
	}
	durable, err := conversation.ReadOwnership(ctx, l.store, owner.Conversation)
	if errors.Is(err, sql.ErrNoRows) {
		return result(proto.RetainedSessionState_RETAINED_SESSION_STATE_STALE)
	}
	if err != nil {
		return nil, status.Error(codes.Unavailable, "retained ownership unavailable")
	}
	if durable.State == "released" {
		return result(proto.RetainedSessionState_RETAINED_SESSION_STATE_RELEASED)
	}
	o := l.owners[owner.Conversation]
	if o == nil || ownerOf(o) != owner || durable.Owner != owner.Host || durable.Epoch != owner.Epoch || durable.State != "owned" {
		return result(proto.RetainedSessionState_RETAINED_SESSION_STATE_STALE)
	}
	if !retained || !open {
		return result(proto.RetainedSessionState_RETAINED_SESSION_STATE_CLOSED)
	}
	return result(proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED)
}
