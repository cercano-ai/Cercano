package server

import (
	"context"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/taskrun"
	pb "cercano/source/server/pkg/proto"
)

type cutoverBlockingProvider struct {
	scriptedProvider
	entered chan struct{}
}

func (p *cutoverBlockingProvider) StreamChat(ctx context.Context, _ llm.ChatRequest) (llm.StreamReader, error) {
	close(p.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestForcedCutoverCancelsScopedDispatchAndReleasesChild(t *testing.T) {
	s, ownedCtx, owner, store := preparedWakeFixture(t)
	s.SetPermissions(agent.NewStaticPermissionStore(agent.ModeBypass), nil)
	if err := s.ConfigurePreparedWorker(preparedWorkerPolicy(t)); err != nil {
		t.Fatal(err)
	}
	registry := agenttools.NewRegistry()
	registry.MustRegister(stubDispatchTool{name: "r_read", perm: agenttools.PermR, retText: "unused"})
	s.SetToolRegistry(registry)
	p := &cutoverBlockingProvider{scriptedProvider: scriptedProvider{caps: inference.Capabilities{SupportsTools: true}}, entered: make(chan struct{})}
	manager, err := taskrun.NewPreparedOwnedCompletion(store, func(ctx context.Context, spec dispatch.Spec) (dispatch.Result, error) {
		return s.runAgenticDispatch(ctx, spec, inference.Selection{Provider: p}, "fixture")
	}, s.notifyOwnedTaskChange, s.coexistence.admission, s.resolvePreparedTaskOwner)
	if err != nil {
		t.Fatal(err)
	}
	s.taskManager = manager
	t.Cleanup(func() { manager.Close(context.Background()) })
	task, err := manager.Launch(ownedCtx, dispatch.Spec{ConversationID: owner.Conversation, Task: "block until forced cutover", Tools: []string{"r_read"}, MaxIterations: 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-p.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// A real scoped dispatch owns a child and parent ticket, not just a fake task.
	child, err := conversation.ReadOwnership(ctx, s.coexistence.store, task.ChildID)
	if err != nil || child.State != "owned" {
		t.Fatalf("child not acquired: %+v %v", child, err)
	}
	b, err := NewCoexistenceServer(ctx, CoexistenceOptions{StorePath: s.coexistence.options.StorePath, Host: "cutover-B", Build: "B", Contract: s.coexistence.options.Contract, Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, o := range b.coexistence.owners {
			o.Close()
		}
		b.coexistence.store.Close()
	})
	ac, _ := serveCutoverFixture(t, s)
	bc, endpoint := serveCutoverFixture(t, b)
	r := &pb.LifecycleControlRequest{Target: s.coexistence.controlIdentity(), Source: s.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), Owner: wireOwner(owner), DestinationEndpoint: endpoint, Management: true, HandoverId: "cancel-scoped-dispatch"}
	result, err := ac.LifecycleForceRelinquish(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	child, err = conversation.ReadOwnership(ctx, s.coexistence.store, task.ChildID)
	if err != nil || child.State != "released" {
		t.Fatalf("child still retained: %+v %v", child, err)
	}
	record, err := manager.Inspect(ctx, owner.Conversation, task.ID)
	if err != nil || record.Status != "stopped" {
		t.Fatalf("outcome %+v %v", record, err)
	}
	r.Target = r.Destination
	r.Receipt = result.Receipt
	transferred, err := bc.LifecycleClaim(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := b.AdmitRetainedWork(controlOwner(&pb.LifecycleControlRequest{Owner: transferred.Owner}), broker.ForegroundWork)
	if err != nil {
		t.Fatal(err)
	}
	ticket.Complete()
}
