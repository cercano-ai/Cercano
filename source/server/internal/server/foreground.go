package server

import (
	"cercano/source/server/internal/broker"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync/atomic"
)

type foregroundKey struct{}
type foregroundWork struct {
	owner      broker.Owner
	attachment *broker.Attachment
	ticket     *broker.WorkTicket
	failed     atomic.Bool
}

// Admission precedes every foreground side effect. The transport attachment is
// only a reference; completing it never releases durable conversation ownership.
func (s *Server) beginForeground(ctx context.Context, conversation string) (context.Context, *foregroundWork, error) {
	if s.coexistence == nil {
		return ctx, nil, nil
	}
	ctx, attachment, ticket, err := s.BeginTerminalStream(ctx, conversation, broker.ForegroundWork)
	if err != nil {
		return nil, nil, err
	}
	owner, _, _ := ParseTerminalBinding(ctx, conversation)
	w := &foregroundWork{owner: owner, attachment: attachment, ticket: ticket}
	ctx = context.WithValue(ctx, foregroundKey{}, w)
	ctx = persistsvc.WithWriteFailure(ctx, func(error) {
		w.failed.Store(true)
		_ = ticket.Advance(broker.PersistenceWork)
		_ = ticket.PersistenceFailed()
	})
	return ctx, w, nil
}
func (w *foregroundWork) check() error {
	if w != nil && w.failed.Load() {
		return status.Error(codes.Internal, "foreground persistence failed; ownership retained")
	}
	return nil
}
func (w *foregroundWork) finish() {
	if w == nil {
		return
	}
	defer w.attachment.Detach()
	_ = w.ticket.Complete()
}

type foregroundStream struct {
	proto.Agent_StreamProcessRequestServer
	ctx  context.Context
	work *foregroundWork
}

func (s *foregroundStream) Context() context.Context { return s.ctx }
func (s *foregroundStream) Send(r *proto.StreamProcessResponse) error {
	if r.GetFinalResponse() != nil {
		if err := s.work.check(); err != nil {
			return err
		}
	}
	return s.Agent_StreamProcessRequestServer.Send(r)
}

// Legacy agent execution needs the same broker exclusion as the tool loop.
func (s *Server) foregroundAgentTurn(ctx context.Context, conversation string, w *foregroundWork) (context.Context, func()) {
	if w == nil {
		return ctx, func() {}
	}
	ctx, _, release := s.beginTurn(ctx, conversation)
	return ctx, func() {
		release()
		if !w.failed.Load() {
			_ = s.notifyOwnedTaskChange(context.WithoutCancel(ctx), w.owner, conversation)
		}
	}
}
