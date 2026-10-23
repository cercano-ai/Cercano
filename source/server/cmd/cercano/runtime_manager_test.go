package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cercano/source/server/internal/localruntime"
	"cercano/source/server/pkg/config"
)

// slowStartManager fakes the Manager interface for the async warm-up test:
// Start blocks until released, recording the request; WriteLog captures
// error entries. Unused methods come from the embedded nil interface.
type slowStartManager struct {
	localruntime.Manager
	started  chan localruntime.StartRequest
	release  chan struct{}
	logged   chan localruntime.LogEntry
	startErr error
}

func (m *slowStartManager) Start(ctx context.Context, req localruntime.StartRequest) (*localruntime.InstanceRecord, error) {
	m.started <- req
	select {
	case <-m.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if m.startErr != nil {
		return nil, m.startErr
	}
	return &localruntime.InstanceRecord{}, nil
}

func (m *slowStartManager) WriteLog(e localruntime.LogEntry) { m.logged <- e }

func TestStartDefaultRuntimeAsyncDoesNotBlockCaller(t *testing.T) {
	m := &slowStartManager{
		started: make(chan localruntime.StartRequest, 1),
		release: make(chan struct{}),
		logged:  make(chan localruntime.LogEntry, 1),
	}
	// Must return while Start is still blocked — a synchronous warm-up here
	// holds the gRPC port unbound for the whole model load (the CLI's
	// auto-launch window is 8s; a GGUF load is comfortably longer).
	startDefaultRuntimeAsync(m, "llama_server", "qwen3-coder-next")
	select {
	case req := <-m.started:
		if req.Runtime != "llama_server" || req.ModelID != "qwen3-coder-next" {
			t.Fatalf("wrong start request: %+v", req)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start was never called")
	}
	close(m.release) // let the background goroutine finish
}

func TestStartDefaultRuntimeAsyncLogsFailure(t *testing.T) {
	m := &slowStartManager{
		started:  make(chan localruntime.StartRequest, 1),
		release:  make(chan struct{}),
		logged:   make(chan localruntime.LogEntry, 1),
		startErr: fmt.Errorf("binary missing"),
	}
	close(m.release) // Start fails immediately
	startDefaultRuntimeAsync(m, "llama_server", "qwen3-coder-next")
	<-m.started
	select {
	case e := <-m.logged:
		if e.Level != "error" || e.Source != "cercano.runtime.llama_server" {
			t.Fatalf("wrong log entry: %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("start failure was never logged")
	}
}

func TestBuildRuntimeManagerRegistersManagedRuntimeCatalogsWhenOllamaActive(t *testing.T) {
	cfg := config.Defaults()
	cfg.OpenRuntime = "ollama"
	cfg.LlamaServer.Enabled = false
	cfg.LlamaServer.DefaultModel = ""
	cfg.MistralRS.Enabled = false
	cfg.MistralRS.DefaultModel = ""

	manager := buildRuntimeManager(cfg, nil) // nil event log: durable recording is a no-op
	models, err := manager.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory returned error: %v", err)
	}
	foundLlamaCatalog := false
	foundMistralCatalog := false
	for _, model := range models {
		// Managed runtimes always expose their curated catalogs, independent of
		// which runtime is active. Runtime switch/readiness/ensure paths resolve
		// against this inventory before the target runtime is warm.
		if model.Runtime == "llama_server" && model.Source == "catalog" {
			foundLlamaCatalog = true
		}
		if model.Runtime == "mistralrs" && model.Source == "catalog" {
			foundMistralCatalog = true
		}
	}
	if !foundLlamaCatalog || !foundMistralCatalog {
		t.Fatalf("expected llama-server and mistral.rs catalog models with ollama active, got %#v", models)
	}
	instances, err := manager.Instances(context.Background())
	if err != nil {
		t.Fatalf("Instances returned error: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("expected no auto-started instances, got %#v", instances)
	}
}

func TestWarmActiveRuntimeStartsEffectiveChatModel(t *testing.T) {
	for _, tc := range []struct {
		runtime string
		set     func(*config.Config)
	}{
		{"llama_server", func(c *config.Config) { c.OpenRuntime = "llama_server"; c.LlamaServer.Enabled = true }},
		{"mistralrs", func(c *config.Config) { c.OpenRuntime = "mistralrs"; c.MistralRS.Enabled = true }},
	} {
		t.Run(tc.runtime, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.LlamaServer.Enabled = false
			cfg.MistralRS.Enabled = false
			tc.set(&cfg)
			cfg.Models.SetOverride(tc.runtime, config.TierEveryday, "warm-me")
			m := &slowStartManager{
				started: make(chan localruntime.StartRequest, 1),
				release: make(chan struct{}),
				logged:  make(chan localruntime.LogEntry, 1),
			}
			close(m.release)
			warmActiveRuntime(m, cfg)
			select {
			case req := <-m.started:
				if req.Runtime != tc.runtime || req.ModelID != "warm-me" {
					t.Fatalf("wrong start request: %+v", req)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Start was never called")
			}
			select {
			case e := <-m.logged:
				if e.Level != "info" || e.Message != warmedMessage(tc.runtime, "warm-me") {
					t.Fatalf("wrong warm log entry: %+v", e)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("successful warm-up was never logged")
			}
		})
	}
}

func TestWarmActiveRuntimeSkipsOllama(t *testing.T) {
	cfg := config.Defaults()
	cfg.OpenRuntime = "ollama"
	cfg.LlamaServer.Enabled = false
	cfg.MistralRS.Enabled = false
	// A nil embedded Manager panics on any call, so this proves no warm-up.
	warmActiveRuntime(&slowStartManager{}, cfg)
}
