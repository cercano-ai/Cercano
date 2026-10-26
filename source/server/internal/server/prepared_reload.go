package server

import (
	"context"
	"fmt"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/engine"
	"cercano/source/server/internal/loop"
	"cercano/source/server/pkg/proto"
)

// NewPreparedDurableServer installs the bounded durable reload bridge on the
// prepared dependency graph. It does not reconcile tasks, replay live work, or
// start generators. Retain/ClaimIncoming remain the explicit acquisition boundary.
func NewPreparedDurableServer(ctx context.Context, o PreparedServerOptions, router RouterCloudUpdater, coordinator *loop.ADKCoordinator, cloudFactory agent.CloudFactory, registry *engine.EngineRegistry) (*Server, error) {
	var s *Server
	o.Coexistence.Reload = nil
	o.Coexistence.StageReload = func(ctx context.Context, store conversation.Store, cap *conversation.Ownership) (func(context.Context) error, error) {
		return s.stagePreparedReload(ctx, store, cap)
	}
	var err error
	s, err = NewPreparedServer(ctx, o, router, coordinator, cloudFactory, registry)
	return s, err
}

// Called with lifecycle.mu held. Only the acquired exact capability may stage
// durable state. The continuation deliberately resolves that same capability,
// never the current owner by ID, after registration and outside lifecycle.mu.
func (s *Server) stagePreparedReload(ctx context.Context, store conversation.Store, cap *conversation.Ownership) (func(context.Context) error, error) {
	if s == nil || s.coexistence == nil || cap == nil || s.persistSvc == nil || s.persistSvc.Store() != store || s.coexistence.store != store || s.coexistence.owners[cap.ConversationID()] != cap || cap.HostInstance() != s.coexistence.options.Host {
		return nil, broker.ErrAdmissionStale
	}
	owner := ownerOf(cap)
	ctx = WithPreparedTaskOwnership(conversation.WithOwnership(ctx, cap), owner, cap)
	if _, err := s.persistSvc.ResumeConversation(ctx, &proto.ResumeConversationRequest{ConversationId: owner.Conversation}); err != nil {
		return nil, err
	}
	tasks, ok := store.(conversation.TaskStore)
	if !ok {
		return nil, fmt.Errorf("prepared store lacks durable task outcomes")
	}
	pending, err := tasks.PendingTasks(ctx, owner.Conversation)
	if err != nil {
		return nil, err
	}
	fresh := false
	for _, task := range pending {
		if !task.WakeClaimed {
			fresh = true
			break
		}
	}
	if !fresh {
		return nil, nil
	}
	return func(context.Context) error {
		if !s.ensureTaskManager() {
			return broker.ErrAdmissionClosed
		}
		return s.notifyOwnedTaskChange(ctx, owner, owner.Conversation)
	}, nil
}
