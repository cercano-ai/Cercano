package main

import (
	"cercano/source/server/internal/agenttools"
	projectctx "cercano/source/server/internal/context"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/engine"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/localruntime"
	mcphost "cercano/source/server/internal/mcp_host"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/toolstack"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
)

// Inference/embedding calls terminate in these in-memory transports. Prepared
// constructor tests use temporary stores and injected credential/runtime factories;
// they do not use native credentials or production runtime services.
type compositionRunner struct {
	name  string
	calls int
}

func (r *compositionRunner) Name() string { return r.name }
func (r *compositionRunner) Process(_ context.Context, _ *agent.Request) (*agent.Response, error) {
	r.calls++
	return &agent.Response{Output: r.name}, nil
}

type compositionEmbedding struct {
	calls int
	model string
	err   error
}

func (e *compositionEmbedding) Name() string { return "memory" }
func (e *compositionEmbedding) Embed(_ context.Context, model, _ string) ([]float64, error) {
	e.calls++
	e.model = model
	return []float64{1, 0}, e.err
}

func TestProductionCompositionLazyRouting(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	// Both callers supply the same graph inputs; lifecycle policy is outside this
	// slice. Explicit memory sessions are created per composition, not globally.
	for _, mode := range []string{"ordinary", "prepared-local"} {
		t.Run(mode, func(t *testing.T) {
			open, cloud := &compositionRunner{name: "open"}, &compositionRunner{name: "cloud"}
			embedding := &compositionEmbedding{}
			graph := composeProductionRouting(productionRoutingDependencies{
				open: open, cloud: cloud, embedder: embedding, embeddingModel: "fixture-embed",
				cloudFactory: func(context.Context, string, string, string, string) (agent.TurnRunner, error) {
					t.Fatal("unexpected provider construction")
					return nil, nil
				},
			})
			if graph.sessions == nil || graph.coordinator == nil || graph.router == nil {
				t.Fatal("incomplete production routing graph")
			}
			if embedding.calls != 0 {
				t.Fatal("composition eagerly invoked embedding transport")
			}
			tiers := graph.router.Tiers()
			if tiers.Open != open || tiers.Cloud != cloud {
				t.Fatal("providers not preserved")
			}
			response, err := tiers.Open.Process(context.Background(), &agent.Request{Input: "fixture"})
			if err != nil || response.Output != "open" || open.calls != 1 || cloud.calls != 0 {
				t.Fatalf("direct open transport: %v, %v", response, err)
			}
			if embedding.calls != 0 {
				t.Fatal("direct lane initialized classifier")
			}
			if _, err := graph.router.ClassifyIntent(&agent.Request{Input: "fixture"}); err != nil {
				t.Fatal(err)
			}
			if embedding.calls == 0 || embedding.model != "fixture-embed" {
				t.Fatal("classifier did not use injected embedding transport/model")
			}
		})
	}
}

func TestProductionCompositionFailureAndIsolation(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	failed := &compositionEmbedding{err: errors.New("fixture transport unavailable")}
	first := composeProductionRouting(productionRoutingDependencies{open: &compositionRunner{name: "first"}, embedder: failed})
	second := composeProductionRouting(productionRoutingDependencies{open: &compositionRunner{name: "second"}, classificationDisabled: true, embedder: &compositionEmbedding{}})
	if first.sessions == second.sessions || first.coordinator == second.coordinator || first.router == second.router {
		t.Fatal("composition shared invocation-local state")
	}
	for i := 0; i < 2; i++ {
		_, err := first.router.ClassifyIntent(&agent.Request{Input: "fixture"})
		if err == nil || !strings.Contains(err.Error(), "fixture transport unavailable") {
			t.Fatalf("embedding failure hidden: %v", err)
		}
	}
	if failed.calls != 1 {
		t.Fatalf("initialization failure retried: %d", failed.calls)
	}
	if _, err := second.router.ClassifyIntent(&agent.Request{Input: "fixture"}); err == nil || !strings.Contains(err.Error(), "local embedding classification is disabled") {
		t.Fatalf("cloud-only policy lost: %v", err)
	}
	replacement := &compositionRunner{name: "replacement"}
	second.router.SetOpenProvider(replacement)
	if first.router.Tiers().Open.Name() != "first" || second.router.Tiers().Open != replacement {
		t.Fatal("provider update leaked across compositions")
	}
}

func TestProductionCompositionServiceParity(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("HOME", t.TempDir())
	routing := composeProductionRouting(productionRoutingDependencies{open: &compositionRunner{name: "memory"}, classificationDisabled: true})
	store, err := conversation.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := agent.NewAgent(routing.router, routing.coordinator, agent.WithPersistentStore(store))
	ordinary := server.NewServer(a, routing.router, routing.coordinator, nil, engine.NewEngineRegistry())
	defer ordinary.Shutdown()
	ordinary.SetSecrets(secrets.NewMemory())
	ordinary.SetOpenLLMProvider(&developmentSyntheticProvider{})
	loader := projectctx.NewLoader()
	bindProductionAgent(ordinary, a, loader, agent.NewStaticPermissionStore(agent.ModeStrict), agent.NewPendingDecisions())
	if err := composeProductionServices(ordinary, a, loader, toolstack.EngineDeps{}, productionExecutionPolicy{legacy: true}); err != nil {
		t.Fatal(err)
	}
	if a.PersistentStore() != store {
		t.Fatal("composition replaced caller store")
	}
	mcpOpens := 0
	o, auth := developmentProductionFixture(t)
	prepared, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{
		mcp:         productionMCPDependencies{newManager: func(*agenttools.Registry, string, time.Duration) *mcphost.Manager { mcpOpens++; return nil }},
		openSecrets: func() (secrets.Store, error) { return secrets.NewMemory(), nil },
		connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func(), error) {
			return localruntime.NewManager(), func() {}, nil
		},
		newOpenProvider: func(localruntime.Manager, config.Config) (inference.Provider, error) {
			return &developmentSyntheticProvider{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer prepared.Shutdown()
	if mcpOpens != 0 {
		t.Fatal("prepared composition opened unsafe MCP integration")
	}
	names := func(s *server.Server) map[string]bool {
		listed, err := s.ListTools(context.Background(), &proto.ListToolsRequest{})
		if err != nil {
			t.Fatal(err)
		}
		result := map[string]bool{}
		for _, tool := range listed.Tools {
			result[tool.Name] = true
		}
		return result
	}
	left, right := names(ordinary), names(prepared)
	if len(left) == 0 || !reflect.DeepEqual(left, right) {
		t.Fatalf("service registrations differ: ordinary=%v prepared=%v", left, right)
	}
}

func TestProductionCompositionPolicyFailsBeforeAssembly(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	for _, policy := range []productionExecutionPolicy{{}, {legacy: true, prepared: &worker.SpawnPolicy{}}, {prepared: &worker.SpawnPolicy{}}} {
		// Nil dependencies would panic if dispatch/capability assembly or worker
		// installation ran before policy validation.
		if err := composeProductionServices(nil, nil, nil, toolstack.EngineDeps{}, policy); err == nil || strings.Contains(err.Error(), "dependencies required") {
			t.Fatalf("policy must fail before service dependencies: %v", err)
		}
		if err := policy.install(nil); err == nil {
			t.Fatal("invalid worker policy accepted")
		}
	}
	o, auth := developmentProductionFixture(t)
	o.Generation = "relative-invalid-generation"
	calls := 0
	_, _, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{
		openSecrets: func() (secrets.Store, error) { calls++; return secrets.NewMemory(), nil },
	})
	if err == nil || calls != 0 {
		t.Fatalf("worker preflight did not precede dependencies: calls=%d err=%v", calls, err)
	}
}
