package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cercano/source/server/internal/agent"
)

// All inference/embedding calls terminate in these in-memory transports. Tests
// never call either host startup (store/runtime/native credential admission).
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
