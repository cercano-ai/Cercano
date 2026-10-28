package server

import (
	"context"
	"errors"
	"fmt"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/engine"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/hostsvc/permissions"
	"cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/loop"
	runnersvc "cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

// PreparedServerOptions constructs the full dependency graph without opening a
// second agent store or loading config/credentials from the machine. Agent is
// called only after read-only contract preflight and must use the supplied store.
// Normal request admission remains disabled pending foreground/context bridging.
type PreparedServerOptions struct {
	// RetainFailure transfers rollback to the caller's existing per-host cleanup
	// handle. It must retain (not execute) disposal until construction returns.
	RetainFailure func(dispose func() error)

	Scheduling               PreparedSchedulingFactory
	Watchdog                 WatchdogDependencies
	CloudBuilder             providers.CloudBuilder
	Coexistence              CoexistenceOptions
	Config                   config.Config
	ConfigPath               string
	Secrets                  secrets.Store
	Agent                    func(conversation.Store) (*agent.Agent, error)
	RoutingPath, FailurePath string
	// WorkerRunner injects a runner using the prepared host services. It must not
	// return nil: explicit injection must never fall back to embedded execution.
	// Callers using this hook must not call the legacy SelectExecutionMode.
	WorkerRunner func(runnersvc.TurnHistory, cfgsvc.Service, permissions.Broker) runnersvc.TurnRunner
}

func NewPreparedServer(ctx context.Context, o PreparedServerOptions, router RouterCloudUpdater, coordinator *loop.ADKCoordinator, cloudFactory agent.CloudFactory, registry *engine.EngineRegistry) (*Server, error) {
	if o.Agent == nil || o.RoutingPath == "" || o.FailurePath == "" {
		return nil, fmt.Errorf("agent factory and explicit diagnostic paths required")
	}
	lifecycle, err := NewCoexistenceServer(ctx, o.Coexistence)
	if err != nil {
		return nil, err
	}
	return newPreparedServerWithLifecycle(o, router, coordinator, cloudFactory, registry, lifecycle)
}

func newPreparedServerWithLifecycle(o PreparedServerOptions, router RouterCloudUpdater, coordinator *loop.ADKCoordinator, cloudFactory agent.CloudFactory, registry *engine.EngineRegistry, lifecycle *Server) (*Server, error) {
	st := lifecycle.coexistence.store
	a, err := o.Agent(st)
	if err != nil {
		return nil, preparedConstructionFailure(o, lifecycle, false, err)
	}
	if a == nil || a.PersistentStore() != st {
		return nil, preparedConstructionFailure(o, lifecycle, false, fmt.Errorf("agent must use the prepared lifecycle store"))
	}
	// Existing schedulers accept only IDs and may launch untracked timers.
	// Disable before exposing the prepared agent; startup must not start generators.
	agent.WithRecapScheduler(nil)(a)
	agent.WithCompactionScheduler(nil)(a)
	agent.WithOwnedScheduling(nil)(a)
	if o.Secrets == nil {
		o.Secrets = secrets.NewMemory()
	} // No native backend fallback.
	cfg := cfgsvc.New(o.ConfigPath, o.Config.WithSharedReadOnly(), secrets.Guard(o.Secrets, secrets.Options{ReadOnly: true}), cfgsvc.Options{Frozen: true})
	s := newServerWithConfig(a, router, coordinator, cloudFactory, registry, o.RoutingPath, o.FailurePath, cfg)
	s.SetCloudProviderBuilder(o.CloudBuilder)
	s.SetWatchdogDependencies(o.Watchdog)
	s.coexistence = lifecycle.coexistence
	if o.WorkerRunner != nil {
		s.workerRunner = o.WorkerRunner(s.persistSvc, s.cfgSvc, s.permBroker)
		if s.workerRunner == nil {
			return nil, preparedConstructionFailure(o, s, true, fmt.Errorf("prepared worker runner factory returned nil"))
		}
	}
	if o.Scheduling != nil {
		s.installPreparedScheduling(a, o.Scheduling)
	}
	return s, nil
}

type preparedTaskAuthority struct {
	owner      broker.Owner
	capability *conversation.Ownership
}
type preparedTaskAuthorityKey struct{}

// WithPreparedTaskOwnership carries explicit launch authority, not a conversation
// ID lookup. This is not a foreground or child-callback admission bridge.
func WithPreparedTaskOwnership(ctx context.Context, owner broker.Owner, capability *conversation.Ownership) context.Context {
	return context.WithValue(ctx, preparedTaskAuthorityKey{}, preparedTaskAuthority{owner, capability})
}
func (s *Server) resolvePreparedTaskOwner(ctx context.Context, id string) (*conversation.Ownership, error) {
	a, ok := ctx.Value(preparedTaskAuthorityKey{}).(preparedTaskAuthority)
	if !ok || a.capability == nil || a.owner.Conversation != id || ownerOf(a.capability) != a.owner {
		return nil, broker.ErrAdmissionStale
	}
	l := s.coexistence
	l.mu.Lock()
	defer l.mu.Unlock()
	if a.owner.Host != l.options.Host || l.owners[id] != a.capability {
		return nil, broker.ErrAdmissionStale
	}
	return a.capability, nil
}

// PreparedConstructionFailure retains the lifecycle's single retirement owner.
// Wait observes cleanup; cancellation never cancels disposal. Error deliberately
// excludes dependency diagnostics, which may contain credentials.
type PreparedConstructionFailure struct {
	cause  error
	server *Server
}

func (e *PreparedConstructionFailure) Error() string {
	return "prepared construction failed; cleanup retained"
}
func (e *PreparedConstructionFailure) Unwrap() error { return e.cause }
func (e *PreparedConstructionFailure) Wait(ctx context.Context) error {
	return errors.Join(e.cause, e.server.RetireCoexistence(ctx))
}
func preparedConstructionFailure(o PreparedServerOptions, s *Server, full bool, cause error) error {
	dispose := func() error {
		var err error
		if full {
			err = s.Shutdown()
		}
		return errors.Join(err, s.CloseCoexistenceStore())
	}
	if o.RetainFailure != nil {
		o.RetainFailure(dispose)
		return cause
	}
	participation := s.coexistence.options.CloseParticipation
	s.coexistence.options.CloseParticipation = func(ctx context.Context) error {
		var err error
		if full {
			err = s.Shutdown()
		}
		return errors.Join(err, participation(ctx))
	}
	failure := &PreparedConstructionFailure{cause: cause, server: s}
	// Retirement retains its asynchronous completion even after this bounded wait.
	return errors.Join(failure, s.RetireCoexistence(context.Background()))
}
