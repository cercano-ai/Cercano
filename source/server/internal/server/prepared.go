package server

import (
	"context"
	"fmt"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/engine"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/loop"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

// PreparedServerOptions constructs the full dependency graph without opening a
// second agent store or loading config/credentials from the machine. Agent is
// called only after read-only contract preflight and must use the supplied store.
// Normal request admission remains disabled pending foreground/context bridging.
type PreparedServerOptions struct {
	Coexistence              CoexistenceOptions
	Config                   config.Config
	ConfigPath               string
	Secrets                  secrets.Store
	Agent                    func(conversation.Store) (*agent.Agent, error)
	RoutingPath, FailurePath string
}

func NewPreparedServer(ctx context.Context, o PreparedServerOptions, router RouterCloudUpdater, coordinator *loop.ADKCoordinator, cloudFactory agent.CloudFactory, registry *engine.EngineRegistry) (*Server, error) {
	if o.Agent == nil || o.RoutingPath == "" || o.FailurePath == "" {
		return nil, fmt.Errorf("agent factory and explicit diagnostic paths required")
	}
	lifecycle, err := NewCoexistenceServer(ctx, o.Coexistence)
	if err != nil {
		return nil, err
	}
	st := lifecycle.coexistence.store
	a, err := o.Agent(st)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	if a == nil || a.PersistentStore() != st {
		_ = st.Close()
		return nil, fmt.Errorf("agent must use the prepared lifecycle store")
	}
	// Existing schedulers accept only IDs and may launch untracked timers.
	// Disable before exposing the prepared agent; startup must not start generators.
	agent.WithRecapScheduler(nil)(a)
	agent.WithCompactionScheduler(nil)(a)
	if o.Secrets == nil {
		o.Secrets = secrets.NewMemory()
	} // No native backend fallback.
	cfg := cfgsvc.New(o.ConfigPath, o.Config.WithSharedReadOnly(), secrets.Guard(o.Secrets, secrets.Options{ReadOnly: true}), cfgsvc.Options{Frozen: true})
	s := newServerWithConfig(a, router, coordinator, cloudFactory, registry, o.RoutingPath, o.FailurePath, cfg)
	s.coexistence = lifecycle.coexistence
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
