package server

import (
	"context"
	"errors"
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
)

type ownedWakeRunner func(context.Context, runner.PersistFunc) (runner.Result, error)

func (f ownedWakeRunner) RunTurn(ctx context.Context, _ runner.Request, _ runner.EventSink, _ runner.PermissionRequester, persist runner.PersistFunc) (runner.Result, error) {
	return f(ctx, persist)
}

func preparedWakeFixture(t *testing.T) (*Server, context.Context, broker.Owner, conversation.TaskStore) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.db")
	if err := conversation.PrepareCoexistence(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	s, err := NewPreparedServer(t.Context(), PreparedServerOptions{
		Coexistence: CoexistenceOptions{StorePath: path, Host: "A", Build: "A", Contract: conversation.CurrentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }},
		Config:      config.Config{OpenRuntime: "synthetic"}, ConfigPath: filepath.Join(dir, "never-read"), RoutingPath: filepath.Join(dir, "routing"), FailurePath: filepath.Join(dir, "failures"),
		Agent: func(st conversation.Store) (*agent.Agent, error) {
			return agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil
		},
	}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine := dispatch.NewEngine(func() inference.Tiers { return inference.Tiers{Open: ownedSyntheticProvider{}} }, func() locus.Mode { return locus.Mode("local") }, nil)
	engine.SetAgenticRunner(func(context.Context, dispatch.Spec, inference.Selection, string) (dispatch.Result, error) {
		return dispatch.Result{Text: "child finished"}, nil
	})
	s.toolSvc.SetEngine(engine)
	owner, err := s.Retain(t.Context(), "parent")
	if err != nil {
		t.Fatal(err)
	}
	cap := s.coexistence.owners["parent"]
	ctx := WithPreparedTaskOwnership(conversation.WithOwnership(t.Context(), cap), owner, cap)
	if err := s.persistSvc.Store().EnsureConversation(ctx, "parent", "", "fixture"); err != nil {
		t.Fatal(err)
	}
	if !s.ensureTaskManager() {
		t.Fatal("manager")
	}
	t.Cleanup(func() {
		s.closeTaskManager()
		_ = cap.Close()
		_ = s.coexistence.store.Close()
		if s.visionStore != nil {
			_ = s.visionStore.Close()
		}
	})
	tasks := s.persistSvc.Store().(conversation.TaskStore)
	record := conversation.TaskRecord{ID: "one", ParentID: "parent", ChildID: "child", Status: "accepted", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := tasks.InsertTask(ctx, record); err != nil {
		t.Fatal(err)
	}
	record.Status = "completed"
	if ok, err := tasks.FinishTask(ctx, record); err != nil || !ok {
		t.Fatal(ok, err)
	}
	return s, ctx, owner, tasks
}

func TestPreparedOwnedCompletionQueue(t *testing.T) {
	for _, mode := range []string{"ack", "failed-write", "failed-ack"} {
		t.Run(mode, func(t *testing.T) {
			fail := mode != "ack"
			s, ctx, owner, tasks := preparedWakeFixture(t)
			if mode == "failed-ack" {
				s.persistSvc = ackFailureService{Service: s.persistSvc, store: ackFailureStore{Store: s.persistSvc.Store(), TaskStore: tasks}}
			}
			entered, finish := make(chan struct{}), make(chan struct{})
			s.inProcessRunner = ownedWakeRunner(func(runctx context.Context, persist runner.PersistFunc) (runner.Result, error) {
				if _, err := s.resolvePreparedTaskOwner(runctx, "parent"); err != nil {
					t.Error(err)
				}
				close(entered)
				<-finish
				if mode == "failed-write" {
					persistsvc.ReportWriteFailure(runctx, errors.New("write failed"))
				} else {
					persist(llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockText, Text: "handled"}}})
				}
				return runner.Result{FinalText: "handled", Model: "fixture"}, nil
			})
			if err := s.notifyOwnedTaskChange(ctx, owner, "parent"); err != nil {
				t.Fatal(err)
			}
			<-entered
			if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("queued callback did not block: %v", err)
			}
			close(finish)
			s.taskWakeWG.Wait()
			pending, err := tasks.PendingTasks(ctx, "parent")
			if err != nil {
				t.Fatal(err)
			}
			if fail {
				if len(pending) != 1 {
					t.Fatal(pending)
				}
				if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("failure lost blocker: %v", err)
				}
			} else {
				if len(pending) != 0 {
					t.Fatal(pending)
				}
				if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
func TestPreparedOwnedCompletionBusyAndStale(t *testing.T) {
	s, ctx, owner, tasks := preparedWakeFixture(t)
	_, _, release := s.beginTurn(ctx, "parent")
	s.inProcessRunner = ownedWakeRunner(func(context.Context, runner.PersistFunc) (runner.Result, error) {
		t.Error("busy/stale ran")
		return runner.Result{}, nil
	})
	if err := s.notifyOwnedTaskChange(ctx, owner, "parent"); err != nil {
		t.Fatal(err)
	}
	s.taskWakeWG.Wait()
	release()
	pending, err := tasks.PendingTasks(ctx, "parent")
	if err != nil || len(pending) != 1 || pending[0].WakeClaimed {
		t.Fatal(pending, err)
	}
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Retain(ctx, "parent"); err != nil {
		t.Fatal(err)
	}
	if err := s.notifyOwnedTaskChange(ctx, owner, "parent"); !errors.Is(err, broker.ErrAdmissionStale) {
		t.Fatal(err)
	}
	pending, err = tasks.PendingTasks(ctx, "parent")
	if err != nil || len(pending) != 1 || pending[0].WakeClaimed {
		t.Fatal(pending, err)
	}
}

// Override only acknowledgement; all claims and terminal writes use the real
// prepared store and its exact capability checks.
type ackFailureStore struct {
	conversation.Store
	conversation.TaskStore
}

func (s ackFailureStore) HandleTaskNotification(context.Context, string, string) error {
	return errors.New("ack failed")
}

type ackFailureService struct {
	persistsvc.Service
	store conversation.Store
}

func (s ackFailureService) Store() conversation.Store { return s.store }

type ownedSyntheticProvider struct{ inference.Provider }

func (ownedSyntheticProvider) Name() string                         { return "synthetic" }
func (ownedSyntheticProvider) Capabilities() inference.Capabilities { return inference.Capabilities{} }
func (ownedSyntheticProvider) Chat(context.Context, inference.Call) (inference.Result, error) {
	return inference.Result{}, nil
}

func TestPreparedOwnedManagerCallback(t *testing.T) {
	s, ctx, owner, tasks := preparedWakeFixture(t)
	// Remove the fixture notice: this wake must originate in the lazy manager.
	if err := tasks.HandleTaskNotification(ctx, "parent", "one"); err != nil {
		t.Fatal(err)
	}
	entered, finish := make(chan struct{}), make(chan struct{})
	s.inProcessRunner = ownedWakeRunner(func(ctx context.Context, persist runner.PersistFunc) (runner.Result, error) {
		close(entered)
		<-finish
		persist(llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockText, Text: "handled"}}})
		return runner.Result{FinalText: "handled", Model: "fixture"}, nil
	})
	task, err := s.taskManager.Launch(ctx, dispatch.Spec{ConversationID: "parent", Task: "synthetic child"})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatal(err)
	}
	close(finish)
	if _, err := s.taskManager.Wait(ctx, "parent", task.ID); err != nil {
		t.Fatal(err)
	}
	s.taskManager.Close(ctx) // wait for the synchronous task completion callback too
	s.taskWakeWG.Wait()
	pending, err := tasks.PendingTasks(ctx, "parent")
	if err != nil || len(pending) != 0 {
		t.Fatal(pending, err)
	}
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
}
