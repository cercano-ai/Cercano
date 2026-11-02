package server

import (
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/pkg/proto"
	"context"
	"database/sql"
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"sync/atomic"
)

type foregroundKey struct{}
type foregroundWork struct {
	cancel     context.CancelFunc
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
	ctx, cancel := context.WithCancel(ctx)
	w := &foregroundWork{owner: owner, attachment: attachment, ticket: ticket, cancel: cancel}
	go func(done <-chan struct{}) {
		select {
		case <-attachment.Cutover():
			cancel()
		case <-done:
		}
	}(ctx.Done())
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
	if w.cancel != nil {
		w.cancel()
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

// Admit against the exact pinned ancestor, never the requested child identity.
// The persistence layer validates ancestry again under its write mutex and
// checks that ancestor's live store-bound capability in the write transaction.
func (s *Server) beginChildVisibility(ctx context.Context, child string) (context.Context, *foregroundWork, error) {
	if s.coexistence == nil {
		return s.beginForeground(ctx, child)
	}
	md, _ := metadata.FromIncomingContext(ctx)
	ids := md.Get("cercano-conversation-id")
	if len(ids) != 1 || child == "" || child == ids[0] {
		return nil, nil, status.Error(codes.InvalidArgument, "child visibility requires pinned ancestor")
	}
	// A terminal task can have failed routing before dispatch created its child
	// conversation. Restore that normal metadata row, not a second visibility
	// store. Only an exact durable parent/task relationship may take this path.
	if _, err := s.persistSvc.Store().Get(ctx, child); errors.Is(err, sql.ErrNoRows) {
		return s.beginMissingTaskVisibility(ctx, ids[0], child)
	}
	// Reject a foreign target before admitting retained work. The store repeats
	// this check transactionally; this early check is not the write authority.
	seen := map[string]bool{}
	for id := child; id != ids[0]; {
		if id == "" || seen[id] {
			return nil, nil, status.Error(codes.PermissionDenied, "unrelated child")
		}
		seen[id] = true
		info, err := s.persistSvc.Store().Get(ctx, id)
		if err != nil || info.Kind != "subagent" {
			return nil, nil, status.Error(codes.PermissionDenied, "unrelated child")
		}
		id = info.ParentID
	}
	ctx, work, err := s.beginForeground(ctx, ids[0])
	if err != nil {
		return nil, nil, err
	}
	return conversation.WithChildVisibilityOwner(ctx, ids[0]), work, nil
}

// Repair only terminal tasks whose execution never acquired the child identity.
// beginPreparedDispatch refuses existing/released/foreign ownership, so closing
// a tab cannot adopt an interrupted execution or compete with a running child.
func (s *Server) beginMissingTaskVisibility(ctx context.Context, parent, child string) (context.Context, *foregroundWork, error) {
	store := s.persistSvc.Store()
	tasks, ok := store.(conversation.TaskStore)
	if !ok {
		return nil, nil, status.Error(codes.PermissionDenied, "unrelated child")
	}
	task, err := tasks.GetTaskByChild(ctx, parent, child)
	if err != nil {
		return nil, nil, status.Error(codes.PermissionDenied, "unrelated child")
	}
	if !task.Terminal() {
		return nil, nil, status.Error(codes.FailedPrecondition, "task is still starting; retry closing its tab after startup finishes")
	}
	ctx, work, err := s.beginForeground(ctx, parent)
	if err != nil {
		return nil, nil, err
	}
	scoped, complete, err := s.beginPreparedDispatch(ctx, parent, child)
	if err != nil {
		work.finish()
		return nil, nil, err
	}
	err = store.EnsureSubagentConversation(scoped, child, parent, task.WorkDir, "", nil)
	err = errors.Join(err, complete(err))
	if err != nil {
		persistsvc.ReportWriteFailure(ctx, err)
		work.finish()
		return nil, nil, err
	}
	return conversation.WithChildVisibilityOwner(ctx, parent), work, nil
}
