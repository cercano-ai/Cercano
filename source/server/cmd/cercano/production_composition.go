package main

import (
	"cercano/source/server/internal/agenttools"
	mcphost "cercano/source/server/internal/mcp_host"
	"context"
	"fmt"
	"sync"
	"time"

	projectctx "cercano/source/server/internal/context"
	"cercano/source/server/internal/locus"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/toolstack"
	"cercano/source/server/internal/worker"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/engine"
	"cercano/source/server/internal/loop"
	"cercano/source/server/internal/tools"
	"google.golang.org/adk/session"
)

// productionRoutingDependencies is invocation-local: callers supply resolved
// providers and embedding transport, never ambient credential/runtime openers.
// Store opening and compatibility preflight remain host responsibilities;
// execution policy below keeps prepared background producers disabled.
type productionRoutingDependencies struct {
	open, cloud    agent.TurnRunner
	cloudFactory   agent.CloudFactory
	embedder       engine.EmbeddingService
	embeddingModel string
	// Prepared cloud-only hosts prohibit local classification. Ordinary startup
	// retains lazy embedding initialization even for MCP-only deployments.
	classificationDisabled bool
}

type productionRouting struct {
	sessions    session.Service
	coordinator *loop.ADKCoordinator
	router      *agent.LazyRouter
}

// composeProductionRouting is the shared routing/validation/session graph, not
// a new startup mode. It performs no I/O and does not enable generation startup.
func composeProductionRouting(d productionRoutingDependencies) productionRouting {
	sessions := session.InMemoryService()
	validator := tools.NewAutoValidator(tools.DefaultLoader(), tools.DefaultKindToValidator())
	coordinator := loop.NewADKCoordinator(d.open, d.cloud, validator, sessions)
	router := agent.NewLazyRouter(func() (*agent.SmartRouter, error) {
		if d.classificationDisabled {
			return nil, fmt.Errorf("cloud_only requires configured managed routing; local embedding classification is disabled")
		}
		return agent.NewSmartRouterFromBytes(d.open, d.cloud, d.embeddingModel, d.embedder, agent.DefaultPrototypes(), d.cloudFactory)
	}, d.open, d.cloud)
	return productionRouting{sessions: sessions, coordinator: coordinator, router: router}
}

// productionExecutionPolicy is explicit: a zero value cannot silently select
// legacy discovery. Prepared policy validation precedes graph mutation.
type productionExecutionPolicy struct {
	legacy   bool
	prepared *worker.SpawnPolicy
}

func (p productionExecutionPolicy) validate() error {
	if p.legacy == (p.prepared != nil) {
		return fmt.Errorf("exactly one production execution policy required")
	}
	if p.prepared != nil {
		return p.prepared.ValidatePrepared()
	}
	return nil
}

func (p productionExecutionPolicy) install(s *server.Server) error {
	if err := p.validate(); err != nil {
		return err
	}
	if p.prepared != nil {
		return s.ConfigurePreparedWorker(*p.prepared)
	}
	s.SelectExecutionMode()
	return nil
}

// Permission policy is supplied by the host: ordinary file-backed decisions
// and prepared strict session-local decisions retain their distinct authority.
func bindProductionAgent(s *server.Server, a *agent.Agent, loader *projectctx.Loader, store *agent.PermissionStore, pending *agent.PendingDecisions) {
	s.SetContextLoader(loader)
	a.SetLocusModeGetter(s.LocusMode)
	s.SetPermissions(store, pending)
}

// composeProductionServices installs the real shared dispatch/capability graph
// onto the caller's exact server and agent; it never opens or replaces a store.
// Worker installation remains a separate final startup step so ordinary MCP
// registration keeps its historical ordering. No background producers are
// enabled for prepared hosts. Accounting sinks are invocation-local inputs.
func composeProductionServices(s *server.Server, a *agent.Agent, loader *projectctx.Loader, accounting toolstack.EngineDeps, policy productionExecutionPolicy) error {
	if err := policy.validate(); err != nil {
		return err
	}
	if s == nil || a == nil || loader == nil {
		return fmt.Errorf("production service dependencies required")
	}
	deps := toolstack.EngineDeps{
		// Resolve providers and models live, never from a captured startup config.
		Providers: s.DispatchCandidates,
		LocusMode: func() locus.Mode { m, _ := locus.ParseMode(s.LocusMode()); return m },
		CtxLoader: loader, ModelFor: s.DispatchModelFor,
		TaskAssignment: s.TaskAssignment, DestinationModelFor: s.DestinationModelFor,
		UsageSink: accounting.UsageSink, AttemptSink: accounting.AttemptSink,
	}
	dispatch := toolstack.NewEngine(deps)
	a.SetDispatchEngine(dispatch)
	s.SetDispatchEngine(dispatch)
	if policy.legacy {
		s.InitWatchdog()
	}
	s.InstallCapabilities()
	return nil
}

// productionMCPDependencies creates a fresh, host-owned manager per invocation.
// The explicit directory is resolved by admission, never by this service factory.
type productionMCPDependencies struct {
	newManager func(*agenttools.Registry, string, time.Duration) *mcphost.Manager
	start      func(*mcphost.Manager, context.Context)
	stop       func(*mcphost.Manager)
}

// composeProductionMCP preserves ordinary register/start ordering and the
// manager's asynchronous, log-only configuration/connect failure behavior.
// Prepared activation remains disabled until shared-definition coordination and
// exclusive integration ownership are safe; registration parity is not a reason
// to launch a second user-configured subprocess.
func composeProductionMCP(s *server.Server, dir string, d productionMCPDependencies, policy productionExecutionPolicy) (func(), error) {
	if err := policy.validate(); err != nil {
		return nil, err
	}
	if !policy.legacy {
		return func() {}, nil
	}
	if s == nil || dir == "" {
		return nil, fmt.Errorf("MCP server and configuration directory required")
	}
	factory := d.newManager
	if factory == nil {
		factory = mcphost.New
	}
	manager := factory(s.ToolRegistry(), dir, 10*time.Second)
	if manager == nil {
		return nil, fmt.Errorf("MCP factory returned nil manager")
	}
	s.SetMcpManager(manager)
	ctx, cancel := context.WithCancel(context.Background())
	start, stop := d.start, d.stop
	if start == nil {
		start = (*mcphost.Manager).Start
	}
	if stop == nil {
		stop = (*mcphost.Manager).Stop
	}
	start(manager, ctx)
	var once sync.Once
	return func() { once.Do(func() { cancel(); stop(manager) }) }, nil
}
