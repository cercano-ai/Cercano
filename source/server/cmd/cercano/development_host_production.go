package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cercano/source/server/internal/agent"
	projectctx "cercano/source/server/internal/context"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/engine"
	llamaengine "cercano/source/server/internal/engine/llamaserver"
	mistralengine "cercano/source/server/internal/engine/mistralrs"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/locus"
	"cercano/source/server/internal/loop"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/tools"
	"cercano/source/server/internal/toolstack"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/devhostauth"
	"google.golang.org/adk/session"
	"gopkg.in/yaml.v3"
)

// Invocation-local seams: tests never open a native credential backend or attach
// to a real runtime. There are deliberately no environment-selected fixtures.
type developmentProductionDependencies struct {
	openSecrets     func() (secrets.Store, error)
	connectRuntime  func(context.Context, config.Config, string, string) (localruntime.Manager, func(), error)
	newOpenProvider func(localruntime.Manager, config.Config) (inference.Provider, error)
}

func newProductionDevelopmentHost(ctx context.Context, o developmentHostOptions, auth *devhostauth.Auth) (*server.Server, func(), error) {
	return newProductionDevelopmentHostWithDependencies(ctx, o, auth, developmentProductionDefaults())
}

func connectDevelopmentRuntime(ctx context.Context, cfg config.Config, state, host string) (localruntime.Manager, func(), error) {
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
	return remote, func() { _ = remote.Close() }, nil
}

func newProductionDevelopmentHostWithDependencies(ctx context.Context, o developmentHostOptions, auth *devhostauth.Auth, deps developmentProductionDependencies) (_ *server.Server, _ func(), err error) {
	// Repeat the read-only contract check so even direct internal callers cannot
	// enter the credential/runtime dependencies before preflight.
	database := filepath.Join(o.State, "conversations.db")
	if err := conversation.PreflightCoexistence(ctx, database, conversation.DevelopmentCoexistenceDeclaration()); err != nil {
		return nil, nil, err
	}
	if o.Identity == nil || auth == nil || deps.openSecrets == nil {
		return nil, nil, fmt.Errorf("prepared identity, auth and dependencies required")
	}
	configPath := filepath.Join(o.State, "config.yaml")
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
	st, err := deps.openSecrets()
	if err != nil {
		return nil, nil, fmt.Errorf("open read-only credentials: %w", err)
	}
	if st == nil {
		return nil, nil, fmt.Errorf("credential opener returned nil store")
	}
	st = secrets.Guard(st, secrets.Options{ReadOnly: true})
	var manager localruntime.Manager
	closeRuntime := func() {}
	if local {
		if deps.connectRuntime == nil {
			return nil, nil, fmt.Errorf("local runtime connector required")
		}
		manager, closeRuntime, err = deps.connectRuntime(ctx, cfg, o.State, o.Host)
		if err != nil {
			return nil, nil, err
		}
		if manager == nil || closeRuntime == nil {
			if closeRuntime != nil {
				closeRuntime()
			}
			return nil, nil, fmt.Errorf("runtime connector returned incomplete dependencies")
		}
	}
	var store conversation.Store
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			closeRuntime()
			if store != nil {
				_ = store.Close()
			}
		})
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
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
	sessions := session.InMemoryService()
	coordinator := loop.NewADKCoordinator(openRunner, cloud, tools.NewAutoValidator(tools.DefaultLoader(), tools.DefaultKindToValidator()), sessions)
	router := agent.NewLazyRouter(func() (*agent.SmartRouter, error) {
		if !local {
			return nil, fmt.Errorf("cloud_only requires configured managed routing; local embedding classification is disabled")
		}
		return agent.NewSmartRouterFromBytes(openRunner, cloud, openEmbeddingModel(cfg), embedder, agent.DefaultPrototypes(), cloudFactory)
	}, openRunner, cloud)
	loader := projectctx.NewLoader()
	var orchestrator *agent.Agent
	diagnostics := filepath.Join(o.State, "hosts", o.Host)
	if err := os.MkdirAll(diagnostics, 0700); err != nil {
		return nil, nil, err
	}
	srv, err := server.NewPreparedDurableServer(ctx, server.PreparedServerOptions{
		Coexistence: server.CoexistenceOptions{ControlAuthorize: auth.Authorize, ControlDial: developmentPeerDial(o.State, o.Identity), StorePath: database, Host: o.Host, Build: o.Identity.Build, Contract: conversation.DevelopmentCoexistenceDeclaration(), CloseParticipation: func(context.Context) error { cleanup(); return nil }},
		Config:      cfg, ConfigPath: configPath, Secrets: st, RoutingPath: filepath.Join(diagnostics, "routing.jsonl"), FailurePath: filepath.Join(diagnostics, "failures.jsonl"),
		Agent: func(prepared conversation.Store) (*agent.Agent, error) {
			store = prepared
			orchestrator = agent.NewAgent(router, coordinator, agent.WithPersistentStore(prepared), agent.WithConversationStore(agent.NewConversationStore(sessions, 3)), agent.WithContextLoader(loader))
			return orchestrator, nil
		},
	}, router, coordinator, cloudFactory, registry)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if err != nil {
			srv.Shutdown()
		}
	}()
	srv.SetBuildVersion(version)
	srv.SetRuntimeManager(manager)
	srv.SetContextLoader(loader)
	orchestrator.SetLocusModeGetter(srv.LocusMode)
	// Prepared hosts must not load/watch the legacy permissions file. Start
	// conservatively with session-local decisions and no persisted allowlist.
	srv.SetPermissions(agent.NewStaticPermissionStore(agent.ModeStrict), agent.NewPendingDecisions())
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
	dispatch := toolstack.NewEngine(toolstack.EngineDeps{
		Providers: srv.DispatchCandidates, LocusMode: func() locus.Mode { m, _ := locus.ParseMode(srv.LocusMode()); return m }, CtxLoader: loader,
		ModelFor: srv.DispatchModelFor, TaskAssignment: srv.TaskAssignment, DestinationModelFor: srv.DestinationModelFor,
	})
	orchestrator.SetDispatchEngine(dispatch)
	srv.SetDispatchEngine(dispatch)
	srv.InstallCapabilities()
	policy, err := worker.NewSpawnPolicy(worker.SpawnOptions{GenerationDir: o.Generation, ManifestPath: filepath.Join(o.Generation, "manifest.json"), HostInstance: o.Host, ResourceRoot: o.RuntimeRoot})
	if err != nil {
		return nil, nil, err
	}
	if err = srv.ConfigurePreparedWorker(policy); err != nil {
		return nil, nil, err
	}
	return srv, cleanup, nil
}
