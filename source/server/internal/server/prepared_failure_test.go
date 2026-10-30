package server

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/conversation"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/hostsvc/permissions"
	runnersvc "cercano/source/server/internal/runner"
)

func TestPreparedConstructorTransfersRollback(t *testing.T) {
	for _, branch := range []string{"agent-error", "invalid-store", "worker-nil"} {
		t.Run(branch, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "fixture.db")
			if err := conversation.PrepareCoexistence(t.Context(), path); err != nil {
				t.Fatal(err)
			}
			original := errors.New("agent sentinel")
			var rollback func() error
			o := PreparedServerOptions{
				Coexistence: CoexistenceOptions{StorePath: path, Host: "host", Build: "build", Contract: conversation.CurrentCoexistenceDeclaration(), CloseParticipation: func(context.Context) error { return nil }},
				RoutingPath: filepath.Join(dir, "routing"), FailurePath: filepath.Join(dir, "failures"),
				RetainFailure: func(f func() error) { rollback = f },
				Agent: func(st conversation.Store) (*agent.Agent, error) {
					if branch == "agent-error" {
						return nil, original
					}
					if branch == "invalid-store" {
						return nil, nil
					}
					return agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil
				},
			}
			if branch == "worker-nil" {
				o.WorkerRunner = func(runnersvc.TurnHistory, cfgsvc.Service, permissions.Broker) runnersvc.TurnRunner { return nil }
			}
			s, err := NewPreparedDurableServer(t.Context(), o, nil, nil, nil, nil)
			if s != nil || err == nil || rollback == nil {
				t.Fatal("rollback not transferred", err)
			}
			if branch == "agent-error" && !errors.Is(err, original) {
				t.Fatal(err)
			}
			if err := rollback(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type preparedBarrierStore struct {
	conversation.Store
	entered, release chan struct{}
	calls            atomic.Int32
	failure          error
}

func (s *preparedBarrierStore) Close() error {
	s.calls.Add(1)
	close(s.entered)
	<-s.release
	return s.failure
}

func TestPreparedFailureRetainsBlockedStoreClose(t *testing.T) {
	for _, branch := range []string{"agent-error", "invalid-store", "worker-nil"} {
		t.Run(branch, func(t *testing.T) {
			s, _, _ := lifecycleFixture(t)
			real := s.coexistence.store
			defer func() { s.coexistence.store = real }()
			sentinel := errors.New("close sentinel")
			original := errors.New("construction sentinel")
			st := &preparedBarrierStore{Store: real, entered: make(chan struct{}), release: make(chan struct{}), failure: sentinel}
			s.coexistence.store = st
			dir := t.TempDir()
			o := PreparedServerOptions{RoutingPath: filepath.Join(dir, "routing"), FailurePath: filepath.Join(dir, "failures"), Agent: func(store conversation.Store) (*agent.Agent, error) {
				if branch == "agent-error" {
					return nil, original
				}
				if branch == "invalid-store" {
					return nil, nil
				}
				return agent.NewAgent(nil, nil, agent.WithPersistentStore(store)), nil
			}}
			if branch == "worker-nil" {
				o.WorkerRunner = func(runnersvc.TurnHistory, cfgsvc.Service, permissions.Broker) runnersvc.TurnRunner { return nil }
			}
			result := make(chan error, 1)
			go func() { _, err := newPreparedServerWithLifecycle(o, nil, nil, nil, nil, s); result <- err }()
			<-st.entered
			var err error
			select {
			case err = <-result:
			case <-time.After(time.Second):
				t.Fatal("constructor blocked on close")
			}
			var retained *PreparedConstructionFailure
			if !errors.As(err, &retained) {
				t.Fatal(err)
			}
			if branch == "agent-error" && !errors.Is(err, original) {
				t.Fatal(err)
			}
			close(st.release)
			for i := 0; i < 2; i++ {
				if e := retained.Wait(context.Background()); !errors.Is(e, sentinel) || (branch == "agent-error" && !errors.Is(e, original)) {
					t.Fatal(e)
				}
			}
			if st.calls.Load() != 1 {
				t.Fatal(st.calls.Load())
			}
		})
	}
}
