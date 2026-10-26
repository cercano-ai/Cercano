package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/locus"
	"cercano/source/server/internal/runner"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

// Demonstrates why an owned completion cannot be scheduled from Reload.
func TestReloadCompletionOrdering(t *testing.T) {
	s, _, _ := lifecycleFixture(t)
	s.coexistence.options.StageReload = func(ctx context.Context, _ conversation.Store, cap *conversation.Ownership) (func(context.Context) error, error) {
		return func(context.Context) error {
			if !s.coexistence.mu.TryLock() {
				return broker.ErrAdmissionBusy
			}
			s.coexistence.mu.Unlock()
			ticket, err := s.coexistence.admission.AdmitWork(ownerOf(cap), broker.CallbackWork)
			if err == nil {
				_ = ticket.Complete()
			}
			return err
		}, nil
	}
	if _, err := s.Retain(context.Background(), "parent"); err != nil {
		t.Fatal(err)
	}
}

func TestStagedReloadReadinessAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			s, _, _ := lifecycleFixture(t)
			entered, finish := make(chan broker.Owner, 1), make(chan struct{})
			s.coexistence.options.StageReload = func(ctx context.Context, _ conversation.Store, cap *conversation.Ownership) (func(context.Context) error, error) {
				return func(context.Context) error {
					entered <- ownerOf(cap)
					<-finish
					if fail {
						return errors.New("schedule failed")
					}
					return nil
				}, nil
			}
			done := make(chan error, 1)
			go func() { _, err := s.Retain(t.Context(), "parent"); done <- err }()
			owner := <-entered
			if _, err := s.AttachRetained(owner); err == nil {
				t.Error("attached before readiness")
			}
			if _, _, err := s.BeginRetainedWork(t.Context(), owner, broker.ForegroundWork); err == nil {
				t.Error("work before readiness")
			}
			if err := s.ReleaseRetained(t.Context(), owner, nil); err == nil {
				t.Error("released while scheduling")
			}
			if err := s.RetireCoexistence(t.Context()); err == nil {
				t.Error("retired while scheduling")
			}
			close(finish)
			err := <-done
			if (err != nil) != fail {
				t.Fatal(err)
			}
			if fail {
				if _, err := s.AttachRetained(owner); err == nil {
					t.Fatal("failure opened admission")
				}
				if err := s.RetireCoexistence(t.Context()); err == nil {
					t.Fatal("failure lost ticket")
				}
			} else {
				a, err := s.AttachRetained(owner)
				if err != nil {
					t.Fatal(err)
				}
				a.Detach()
			}
		})
	}
}

func TestPreparedDurableReloadPendingAndSibling(t *testing.T) {
	a, ctx, old, tasks := preparedWakeFixture(t)
	// More than one completion batch: no pending outcome may lose wake coverage.
	for i := 2; i <= 65; i++ {
		task := conversation.TaskRecord{ID: fmt.Sprint(i), ParentID: "parent", ChildID: fmt.Sprint("child", i), Status: "accepted", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := tasks.InsertTask(ctx, task); err != nil {
			t.Fatal(err)
		}
		task.Status = "completed"
		if ok, err := tasks.FinishTask(ctx, task); err != nil || !ok {
			t.Fatal(ok, err)
		}
	}
	sibling, err := a.Retain(ctx, "sibling")
	if err != nil {
		t.Fatal(err)
	}
	siblingCtx, ticket, err := a.BeginRetainedWork(ctx, sibling, broker.ForegroundWork)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.persistSvc.Store().EnsureConversation(siblingCtx, "sibling", "", "sibling"); err != nil {
		t.Fatal(err)
	}
	live := conversation.TaskRecord{ID: "live", ParentID: "sibling", ChildID: "live-child", Status: "accepted", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := tasks.InsertTask(siblingCtx, live); err != nil {
		t.Fatal(err)
	}
	defer ticket.Complete()
	if err := a.ReleaseRetained(ctx, old, nil); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	b, err := NewPreparedDurableServer(t.Context(), PreparedServerOptions{
		Coexistence: CoexistenceOptions{StorePath: a.coexistence.options.StorePath, Host: "B", Build: "B", Contract: conversation.CurrentCoexistenceDeclaration(), CloseParticipation: func(context.Context) error { return nil }},
		Config:      config.Config{OpenRuntime: "synthetic"}, ConfigPath: filepath.Join(dir, "config"), RoutingPath: filepath.Join(dir, "routing"), FailurePath: filepath.Join(dir, "failures"),
		Agent: func(st conversation.Store) (*agent.Agent, error) {
			return agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil
		},
	}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		b.closeTaskManager()
		for _, cap := range b.coexistence.owners {
			_ = cap.Close()
		}
		_ = b.coexistence.store.Close()
		if b.visionStore != nil {
			_ = b.visionStore.Close()
		}
	})
	engine := dispatch.NewEngine(func() inference.Tiers { return inference.Tiers{Open: ownedSyntheticProvider{}} }, func() locus.Mode { return locus.Mode("local") }, nil)
	b.toolSvc.SetEngine(engine)
	b.inProcessRunner = ownedWakeRunner(func(runctx context.Context, persist runner.PersistFunc) (runner.Result, error) {
		cap, err := b.resolvePreparedTaskOwner(runctx, "parent")
		if err != nil || cap.HostInstance() != "B" {
			t.Errorf("wrong exact owner: %v", err)
		}
		if _, err := b.resolvePreparedTaskOwner(ctx, "parent"); err == nil {
			t.Error("accepted A capability")
		}
		persist(llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockText, Text: "handled"}}})
		return runner.Result{FinalText: "handled", Model: "fixture"}, nil
	})
	if _, err := b.Retain(t.Context(), "parent"); err != nil {
		t.Fatal(err)
	}
	b.taskWakeWG.Wait()
	pending, err := tasks.PendingTasks(ctx, "parent")
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending=%d err=%v", len(pending), err)
	}
	// B's reload must not reconcile A's still-live sibling task.
	live.Status = "completed"
	if ok, err := tasks.FinishTask(siblingCtx, live); err != nil || !ok {
		t.Fatalf("sibling changed: %v %v", ok, err)
	}
}

func TestPreparedDurableReloadFailureClosed(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
	s.persistSvc = reloadFailureService{Service: s.persistSvc}
	s.coexistence.options.StageReload = s.stagePreparedReload
	if _, err := s.Retain(ctx, "parent"); err == nil {
		t.Fatal("reload unexpectedly succeeded")
	}
	cap := s.coexistence.owners["parent"]
	if cap == nil {
		t.Fatal("lost exact capability")
	}
	defer cap.Close()
	if _, err := s.AttachRetained(ownerOf(cap)); err == nil {
		t.Fatal("reload failure admitted attachment")
	}
	if _, _, err := s.BeginRetainedWork(ctx, ownerOf(cap), broker.ForegroundWork); err == nil {
		t.Fatal("reload failure admitted work")
	}
	if err := s.RetireCoexistence(ctx); err == nil {
		t.Fatal("reload failure lost retirement blockers")
	}
}

type reloadFailureService struct{ persistsvc.Service }

func (reloadFailureService) ResumeConversation(context.Context, *proto.ResumeConversationRequest) (*proto.ResumeConversationResponse, error) {
	return nil, errors.New("durable reload unavailable")
}
