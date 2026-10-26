package server

import (
	"cercano/source/server/internal/broker"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ParseTerminalBinding accepts exactly one of each pinned field and only the
// named conversation. Metadata identifies a caller; it does not grant ownership.
func ParseTerminalBinding(ctx context.Context, conversation string) (broker.Owner, string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	keys := []string{"cercano-host-instance", "cercano-conversation-id", "cercano-ownership-epoch", "cercano-terminal-attachment"}
	values := make([]string, len(keys))
	for i, key := range keys {
		v := md.Get(key)
		if len(v) != 1 || v[0] == "" {
			return broker.Owner{}, "", status.Error(codes.InvalidArgument, "missing or duplicate terminal binding")
		}
		values[i] = v[0]
	}
	if conversation == "" || values[1] != conversation {
		return broker.Owner{}, "", status.Error(codes.InvalidArgument, "terminal conversation mismatch")
	}
	return broker.Owner{Host: values[0], Conversation: values[1], Epoch: values[2]}, values[3], nil
}

// BeginTerminalStream enrolls one existing conversation stream. kind==0 is an
// observer; otherwise the caller owns the returned work ticket through durable
// completion (NOT merely stream disconnect). Always detach the attachment.
// Legacy servers return their unchanged context and nil handles.
func (s *Server) BeginTerminalStream(ctx context.Context, conversation string, kind broker.WorkKind) (context.Context, *broker.Attachment, *broker.WorkTicket, error) {
	if s.coexistence == nil {
		return ctx, nil, nil, nil
	}
	owner, id, err := ParseTerminalBinding(ctx, conversation)
	if err != nil {
		return nil, nil, nil, err
	}
	l, err := s.lifecycle()
	if err != nil {
		return nil, nil, nil, err
	}
	l.mu.Lock()
	if !l.ready[owner] {
		l.mu.Unlock()
		return nil, nil, nil, broker.ErrAdmissionBusy
	}
	attachment, err := l.admission.AttachTerminal(owner, id)
	l.mu.Unlock()
	if err != nil {
		return nil, nil, nil, err
	}
	if kind == 0 {
		return ctx, attachment, nil, nil
	}
	workCtx, ticket, err := s.BeginRetainedWork(ctx, owner, kind)
	if err != nil {
		attachment.Detach()
		return nil, nil, nil, err
	}
	return workCtx, attachment, ticket, nil
}

// ValidateTerminalPane never enrolls a terminal or admits work. Stream wrappers
// must use this only for read-only pane subscriptions, not execution authority.
func (s *Server) ValidateTerminalPane(ctx context.Context, conversation string) error {
	if s.coexistence == nil {
		return nil
	}
	owner, id, err := ParseTerminalBinding(ctx, conversation)
	if err != nil {
		return err
	}
	l, err := s.lifecycle()
	if err != nil {
		return err
	}
	return l.admission.ValidateTerminal(owner, id)
}

// CloseTerminalOwner atomically resolves a live terminal requester and closes
// admission. Management callers without a terminal use Admission.CloseTerminal
// with an empty ID; missing transport metadata is never management authority.
func (s *Server) CloseTerminalOwner(ctx context.Context, conversation string) (*broker.Closure, error) {
	owner, id, err := ParseTerminalBinding(ctx, conversation)
	if err != nil {
		return nil, err
	}
	l, err := s.lifecycle()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.ready[owner] {
		return nil, broker.ErrAdmissionBusy
	}
	return l.admission.CloseTerminal(owner, id)
}
