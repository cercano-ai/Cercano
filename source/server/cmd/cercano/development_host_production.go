package main

import (
	"context"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cercano/source/server/internal/agent"
	projectctx "cercano/source/server/internal/context"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/engine"
	llamaengine "cercano/source/server/internal/engine/llamaserver"
	mistralengine "cercano/source/server/internal/engine/mistralrs"
	"cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/locus"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/toolstack"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/devhostauth"
)

// Invocation-local seams: tests never open a native credential backend or attach
// to a real runtime. There are deliberately no environment-selected fixtures.
type developmentProductionDependencies struct {
	watchdog        server.WatchdogDependencies
	cloudBuilder    providers.CloudBuilder
	mcp             productionMCPDependencies
	openSecrets     func() (secrets.Store, error)
	connectRuntime  func(context.Context, config.Config, string, string) (localruntime.Manager, func() error, error)
	newOpenProvider func(localruntime.Manager, config.Config) (inference.Provider, error)
}

func connectDevelopmentRuntime(ctx context.Context, cfg config.Config, state, host string) (localruntime.Manager, func() error, error) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(modelHostEnv)), "inprocess") {
		return nil, nil, fmt.Errorf("development host requires protected shared modelhost; inprocess is forbidden")
	}
	// A separate state resource pins attachment; never discover the ambient host.
	attachmentPath := filepath.Join(state, "modelhost.yaml")
	if err := developmentPrivatePath(attachmentPath, false); err != nil {
		return nil, nil, fmt.Errorf("local runtime requires private prepared modelhost.yaml: %w", err)
	}
	data, err := os.ReadFile(attachmentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("local runtime requires prepared modelhost.yaml: %w", err)
	}
	var attachment struct {
		RunDir string `yaml:"run_dir"`
	}
	if err := yaml.Unmarshal(data, &attachment); err != nil {
		return nil, nil, err
	}
	if err := developmentPrivatePath(attachment.RunDir, true); err != nil {
		return nil, nil, fmt.Errorf("prepared modelhost run_dir: %w", err)
	}
	bounded, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	remote, err := modelhost.NewRemote(bounded, modelhost.RemoteOptions{
		CoexistenceProtection: true, ClientID: host, Profile: "development",
		Ensure: modelhost.EnsureOptions{RunDir: attachment.RunDir, CoexistenceProtection: true, ClientVersion: version},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("attach protected modelhost (no launch/replacement/fallback): %w", err)
	}
	remote.SetEndpoints(localruntime.EndpointsFromConfig(cfg, openChatModel(cfg), openEmbeddingModel(cfg)))
	return remote, remote.Close, nil
}

func newProductionDevelopmentHostWithDependencies(ctx context.Context, o developmentHostOptions, auth *devhostauth.Auth, deps developmentProductionDependencies) (_ *server.Server, pending *preparedShutdown, err error) {
	// Repeat the read-only contract check so even direct internal callers cannot
	// enter the credential/runtime dependencies before preflight.
	database := filepath.Join(o.State, "conversations.db")
	startup := canonicalGenerationStartup()
	// Existing prepared entry retains its explicitly selected snapshot authority;
	// canonical generation policy never falls back to these paths.
	startup.authorities = productionAuthorityResolvers{
		configPath:       func() string { return filepath.Join(o.State, "config.yaml") },
		conversationPath: func() (string, error) { return database, nil },
	}
	admission, err := startup.admit(ctx)
	if err != nil {
		return nil, nil, err
	}
	if o.Identity == nil || auth == nil || deps.openSecrets == nil {
		return nil, nil, fmt.Errorf("prepared identity, auth and dependencies required")
	}
	configPath := admission.paths.config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, err
	}
	// Intentionally not config.Load: that merges machine environment overrides
	// and migrates legacy profiles. This host consumes only the prepared snapshot.
	cfg := config.Defaults()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("prepared config: %w", err)
	}
	if err := validateDevelopmentCloudProfiles(cfg); err != nil {
		return nil, nil, err
	}
	cfg = cfg.WithSharedReadOnly()
	if _, err := locus.ParseMode(cfg.LocusMode); err != nil {
		return nil, nil, fmt.Errorf("prepared locus_mode: %w", err)
	}
	if cfg.LocusMode != "open_only" && cfg.ActiveCloudProfile == "" {
		return nil, nil, fmt.Errorf("cloud routing selected without an active prepared cloud profile")
	}
	local := cfg.LocusMode != "cloud_only"
	runtime := strings.ToLower(cfg.OpenRuntime)
	if local && runtime != "llama_server" && runtime != "mistralrs" {
		return nil, nil, fmt.Errorf("unsupported development open_runtime %q: protected llama_server or mistralrs required", cfg.OpenRuntime)
	}
	if err := cfg.LlamaServer.Validate(); local && err != nil {
		return nil, nil, err
	}
	if !config.ValidateToolLoopMaxIterations(cfg.ToolLoop.MaxIterations) {
		return nil, nil, fmt.Errorf("invalid tool loop iteration limit")
	}
	policy, err := worker.NewSpawnPolicy(worker.SpawnOptions{GenerationDir: o.Generation, ManifestPath: filepath.Join(o.Generation, "manifest.json"), HostInstance: o.Host, ResourceRoot: o.RuntimeRoot})
	if err != nil {
		return nil, nil, err
	}
	executionPolicy := productionExecutionPolicy{prepared: &policy}

	if err := executionPolicy.validate(); err != nil {
		return nil, nil, err
	}
	st, err := (productionCredentialDependencies{openSecrets: deps.openSecrets}).open(secrets.Options{ReadOnly: true})
	if err != nil {
		return nil, nil, fmt.Errorf("open read-only credentials: %w", err)
	}
	if st == nil {
		return nil, nil, fmt.Errorf("credential opener returned nil store")
	}
	st = secrets.Guard(st, secrets.Options{ReadOnly: true})
	var manager localruntime.Manager
	closeRuntime := func() error { return nil }
	closeMCP := func() {}
	var store conversation.Store
	var constructorRollback func() error
	resources := newPreparedShutdown(nil, func() error {
		closeMCP()
		var err error
		if closeRuntime != nil {
			err = closeRuntime()
		}
		return err
	})
	ctx, cancel := context.WithCancel(ctx)
	var ownedServer *server.Server
	cleanup := newPreparedShutdown(func() {
		if ownedServer != nil {
			ownedServer.FenceBackgroundWork()
		}
	}, func() error {
		cancel()
		var shutdownErr error
		if ownedServer != nil {
			ownedServer.BeginShutdown()
			ownedServer.WaitForWatchers()
			shutdownErr = ownedServer.Shutdown()
		}
		err := errors.Join(shutdownErr, resources.Wait(context.Background()))
		if constructorRollback != nil {
			err = errors.Join(err, constructorRollback())
		}
		if ownedServer != nil {
			err = errors.Join(err, ownedServer.CloseCoexistenceStore())
		}
		return err
	})
	defer func() {
		if err != nil {
			pending = cleanup
			err = finishPreparedShutdown(cleanup, err)
		}
	}()
	if local {
		if deps.connectRuntime == nil {
			return nil, nil, fmt.Errorf("local runtime connector required")
		}
		manager, closeRuntime, err = deps.connectRuntime(ctx, cfg, o.State, o.Host)
		if err != nil {
			return nil, nil, err
		}
		if manager == nil || closeRuntime == nil {
			return nil, nil, fmt.Errorf("runtime connector returned incomplete dependencies")
		}
	}
	registry := engine.NewEngineRegistry()
	var open inference.Provider
	var embedder engine.EmbeddingService
	var setFailureLog func(*server.Server)
	if local {
		llama := llamaengine.NewEngine(manager)
		setFailureLog = func(s *server.Server) { llama.SetFailureLog(s.FailureLog()) }
		mistral := mistralengine.NewEngine(manager)
		registry.RegisterEngine(llama)
		registry.RegisterEmbedder(llama)
		registry.RegisterEngine(mistral)
		embedder = llama
		if deps.newOpenProvider != nil {
			open, err = deps.newOpenProvider(manager, cfg)
			if err != nil {
				return nil, nil, err
			}
			if open == nil {
				return nil, nil, fmt.Errorf("provider constructor returned nil local provider")
			}
		} else if runtime == "mistralrs" {
			open = mistralengine.NewLLMProvider(mistral)
		} else {
			open = llamaengine.NewLLMProvider(llama)
		}
	}
	// An unavailable local lane is an explicit error, never a cloud fallback.
	var openRunner agent.TurnRunner = agent.AbsentCloud("local lane disabled by cloud_only configuration")
	if open != nil {
		openRunner = agent.InferenceTurnRunner(open, openTurnModel(cfg))
	}
	cloud := agent.AbsentCloud("prepared cloud profile not configured")
	cloudFactory := developmentCloudFactory(cfg, st)
	routing := composeProductionRouting(productionRoutingDependencies{
		open: openRunner, cloud: cloud, cloudFactory: cloudFactory,
		embeddingModel: openEmbeddingModel(cfg), embedder: embedder, classificationDisabled: !local,
	})
	coordinator, router := routing.coordinator, routing.router

	var loader *projectctx.Loader
	var orchestrator *agent.Agent
	diagnostics := filepath.Join(o.State, "hosts", o.Host)
	if err := os.MkdirAll(diagnostics, 0700); err != nil {
		return nil, nil, err
	}
	srv, err := server.NewPreparedDurableServer(ctx, server.PreparedServerOptions{
		RetainFailure: func(dispose func() error) { constructorRollback = dispose },
		Watchdog:      deps.watchdog,
		CloudBuilder:  deps.cloudBuilder,
		Coexistence:   server.CoexistenceOptions{ControlAuthorize: auth.Authorize, ControlDial: developmentPeerDial(o.State, o.Identity), StorePath: database, Host: o.Host, Build: o.Identity.Build, Contract: conversation.DevelopmentCoexistenceDeclaration(), CloseParticipation: func(ctx context.Context) error { return resources.Wait(ctx) }},
		Config:        cfg, ConfigPath: configPath, Secrets: st, RoutingPath: filepath.Join(diagnostics, "routing.jsonl"), FailurePath: filepath.Join(diagnostics, "failures.jsonl"),
		Agent: func(prepared conversation.Store) (*agent.Agent, error) {
			store, err = admission.conversationStore(prepared, nil)
			if err != nil {
				return nil, err
			}
			orchestrator, loader, err = composeProductionAgent(routing, store, cfg, productionScheduling{})
			return orchestrator, err
		},
	}, router, coordinator, cloudFactory, registry)
	if err != nil {
		return nil, nil, err
	}
	ownedServer = srv
	srv.SetBuildVersion(version)
	srv.SetRuntimeManager(manager)
	// Prepared hosts must not load/watch the legacy permissions file. Start
	// conservatively with session-local decisions and no persisted allowlist.
	bindProductionAgent(srv, orchestrator, loader, agent.NewStaticPermissionStore(agent.ModeStrict), agent.NewPendingDecisions())
	srv.SetOpenLLMProvider(open)
	if setFailureLog != nil {
		setFailureLog(srv)
	}
	// Failure of a configured route is fatal, never an implicit open-lane fallback.
	if cfg.ActiveCloudProfile != "" || len(cfg.CloudProfiles) > 0 {
		if err = srv.RebuildCloud(); err != nil {
			return nil, nil, fmt.Errorf("prepared cloud routes: %w", err)
		}
	} else if cfg.LocusMode != "open_only" {
		return nil, nil, fmt.Errorf("cloud routing selected without an active prepared cloud profile")
	}
	if err = composeProductionServices(srv, orchestrator, loader, toolstack.EngineDeps{}, executionPolicy); err != nil {
		return nil, nil, err
	}
	mcpCleanup, err := composeProductionMCP(srv, filepath.Dir(configPath), deps.mcp, executionPolicy)
	if err != nil {
		return nil, nil, err
	}
	closeMCP = mcpCleanup
	if err = executionPolicy.install(srv); err != nil {
		return nil, nil, err
	}
	return srv, cleanup, nil
}
