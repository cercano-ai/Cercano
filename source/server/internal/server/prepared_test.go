package server

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/hostsvc/permissions"
	runnersvc "cercano/source/server/internal/runner"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

func TestPreparedFullServerPreservesPeerTasks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.db")
	if err := conversation.PrepareCoexistence(ctx, path); err != nil {
		t.Fatal(err)
	}
	makeServer := func(host string) *Server {
		s, err := NewPreparedServer(ctx, PreparedServerOptions{
			Coexistence: CoexistenceOptions{StorePath: path, Host: host, Build: host, Contract: conversation.CurrentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }},
			Config:      config.Config{OpenRuntime: "synthetic"}, ConfigPath: filepath.Join(dir, "never-read.yaml"),
			RoutingPath: filepath.Join(dir, host+"-routing"), FailurePath: filepath.Join(dir, host+"-failures"),
			Agent: func(st conversation.Store) (*agent.Agent, error) {
				return agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil
			},
		}, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if s.persistSvc.Store() != s.coexistence.store || s.agent.PersistentStore() != s.coexistence.store {
			t.Fatal("split stores")
		}
		s.toolSvc.SetEngine(&dispatch.Engine{})
		t.Cleanup(func() {
			s.closeTaskManager()
			for _, o := range s.coexistence.owners {
				_ = o.Close()
			}
			_ = s.coexistence.store.Close()
			if s.visionStore != nil {
				_ = s.visionStore.Close()
			}
		})
		return s
	}
	a := makeServer("A")
	owner, err := a.Retain(ctx, "parent")
	if err != nil {
		t.Fatal(err)
	}
	cap := a.coexistence.owners["parent"]
	owned := conversation.WithOwnership(ctx, cap)
	if err := a.persistSvc.Store().EnsureConversation(owned, "parent", "", "model"); err != nil {
		t.Fatal(err)
	}
	tasks := a.persistSvc.Store().(conversation.TaskStore)
	for _, id := range []string{"accepted", "running"} {
		if err := tasks.InsertTask(owned, conversation.TaskRecord{ID: id, ParentID: "parent", Status: "accepted", Task: "synthetic", ChildID: id + "-child", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tasks.StartTask(owned, "parent", "running"); err != nil {
		t.Fatal(err)
	}
	before, err := tasks.ListTasks(ctx, "parent", 10)
	if err != nil {
		t.Fatal(err)
	}
	b := makeServer("B")
	// OpenCoexistence rejects InitTasks and ReconcileTasks. Success proves the
	// lazy real-server manager never calls either global startup method.
	if !b.ensureTaskManager() || !b.ensureTaskManager() {
		t.Fatal("prepared manager not constructed")
	}
	after, err := tasks.ListTasks(ctx, "parent", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("peer startup changed tasks: before=%+v after=%+v", before, after)
	}
	if _, err := a.resolvePreparedTaskOwner(ctx, "parent"); !errors.Is(err, broker.ErrAdmissionStale) {
		t.Fatal("guessed ownership", err)
	}
	exact := WithPreparedTaskOwnership(ctx, owner, cap)
	if got, err := a.resolvePreparedTaskOwner(exact, "parent"); err != nil || got != cap {
		t.Fatal(got, err)
	}
	if _, err := b.resolvePreparedTaskOwner(exact, "parent"); !errors.Is(err, broker.ErrAdmissionStale) {
		t.Fatal("accepted foreign capability", err)
	}
	// Invalid URL would reach runtime/provider effects without the entry guard.
	if _, err := b.UpdateConfig(ctx, &proto.UpdateConfigRequest{OllamaUrl: "not-a-url"}); !errors.Is(err, cfgsvc.ErrFrozen) {
		t.Fatal(err)
	}
	if err := b.StartConfigWatcher(ctx, filepath.Join(dir, "missing", "config")); !errors.Is(err, cfgsvc.ErrFrozen) {
		t.Fatal(err)
	}
	if err := b.SetConfigPersistence("changed", config.Config{}); !errors.Is(err, cfgsvc.ErrFrozen) {
		t.Fatal(err)
	}
	if err := b.SetSecrets(nil); !errors.Is(err, cfgsvc.ErrFrozen) {
		t.Fatal(err)
	}
	if b.cfgSvc.Get().OpenRuntime != "synthetic" {
		t.Fatal("initial snapshot lost")
	}
	b.reloadConfigFromDisk(ctx)
}

// Selection-only synthetic runner: no turn may execute in this fixture.
type preparedSyntheticRunner struct{ runnersvc.TurnRunner }

func TestPreparedWorkerRunnerInjection(t *testing.T) {
	for _, inject := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "injected"}[inject], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "fixture.db")
			if err := conversation.PrepareCoexistence(t.Context(), path); err != nil {
				t.Fatal(err)
			}
			want := &preparedSyntheticRunner{}
			called := false
			o := PreparedServerOptions{
				Coexistence: CoexistenceOptions{StorePath: path, Host: "host", Build: "build", Contract: conversation.CurrentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }},
				RoutingPath: filepath.Join(dir, "routing"), FailurePath: filepath.Join(dir, "failures"),
				Agent: func(st conversation.Store) (*agent.Agent, error) {
					return agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil
				},
			}
			if inject {
				o.WorkerRunner = func(history runnersvc.TurnHistory, cfg cfgsvc.Service, perms permissions.Broker) runnersvc.TurnRunner {
					called = true
					if history == nil || cfg == nil {
						t.Fatal("missing prepared services")
					}
					return want
				}
			}
			s, err := NewPreparedServer(t.Context(), o, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer s.coexistence.store.Close()
			defer s.Shutdown()
			if inject {
				if !called || s.pickTurnRunner() != want {
					t.Fatal("supplied runner not selected")
				}
			} else if s.workerRunner != nil || s.pickTurnRunner() != s.inProcessRunner {
				t.Fatal("uninjected default changed")
			}
		})
	}
}

func TestPreparedFullServerPreflightBeforeDependencies(t *testing.T) {
	dir := t.TempDir()
	called := false
	_, err := NewPreparedServer(context.Background(), PreparedServerOptions{
		Coexistence: CoexistenceOptions{StorePath: filepath.Join(dir, "absent.db"), Host: "host", Build: "build", Contract: conversation.CurrentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }},
		RoutingPath: filepath.Join(dir, "routing"), FailurePath: filepath.Join(dir, "failures"),
		Agent: func(conversation.Store) (*agent.Agent, error) { called = true; return nil, nil },
	}, nil, nil, nil, nil)
	if err == nil || called {
		t.Fatalf("preflight err=%v factoryCalled=%v", err, called)
	}
}
