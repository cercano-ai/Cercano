package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

// Bypass only the credential facade in this fixture: native credentials remain
// disabled and reset's cross-store barrier operates exclusively on memory.
type counterexampleConfig struct {
	cfgsvc.Service
	store secrets.Store
}

func (c counterexampleConfig) Secrets() secrets.Store { return c.store }

type counterexampleDelete struct {
	secrets.Store
	entered, release chan struct{}
}

func (s *counterexampleDelete) Delete(name string) error {
	close(s.entered)
	<-s.release
	return s.Store.Delete(name)
}

type counterexampleProviders struct {
	providers.ComposableResolver
	router *counterexampleRouter
	calls  int
	last   providers.ReconfigureArgs
}

func (p *counterexampleProviders) Reconfigure(a providers.ReconfigureArgs) {
	p.calls++
	p.last = a
	p.ComposableResolver.Reconfigure(a)
}

func counterexampleServer(t *testing.T) (*Server, *counterexampleProviders) {
	t.Helper()
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	dir := t.TempDir()
	router := &counterexampleRouter{}
	s := newServerWithLogs(nil, router, nil, nil, nil, filepath.Join(dir, "routing.jsonl"), filepath.Join(dir, "failures.jsonl"))
	path := filepath.Join(dir, "config.yaml")
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c.OpenRuntime = "llama_server"
	c.Models.SetOverride("llama_server", config.TierEveryday, "old-model")
	next, err := config.SaveSnapshot(c, path, c.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	c = c.WithSnapshot(next)
	s.SetConfigPersistence(path, c)
	s.cfgSvc = counterexampleConfig{s.cfgSvc, secrets.NewMemory()}
	s.SetOpenProviderFactory(func(config.Config) inference.Provider { return &counterexampleInference{} })
	p := &counterexampleProviders{ComposableResolver: s.providerSvc, router: router}
	s.providerSvc = p
	router.count = 0
	return s, p
}

func TestConfigApplyCounterexampleOverrideRemoval(t *testing.T) {
	s, p := counterexampleServer(t)
	c := s.cfgSvc.Get()
	c.Models.SetOverride(c.OpenRuntime, config.TierEveryday, "")
	next, err := config.SaveSnapshot(c, s.cfgSvc.Path(), c.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	s.reloadConfigFromDisk(context.Background())
	if s.cfgSvc.Get().Snapshot().Revision != next.Revision {
		t.Fatal("snapshot not adopted")
	}
	if p.calls != 1 {
		t.Fatalf("runner reconfigure count=%d, want 1 for cleared override", p.calls)
	}
	if p.router.count != 1 || p.router.open == nil {
		t.Fatal("open runner not replaced")
	}
	response, err := p.router.open.Process(context.Background(), &agent.Request{Input: "fixture"})
	if err != nil || response.Output != effectiveOpenModelFor(c, config.TierEveryday) {
		t.Fatalf("runner retained removed model: response=%+v err=%v", response, err)
	}
	if p.last.OpenRuntime != c.OpenRuntime || p.last.ResolvedOpenModel != effectiveOpenModelFor(c, config.TierEveryday) {
		t.Fatalf("stale runner selection: %+v", p.last)
	}
	if v, _ := p.last.MutatedConfig.Models.OverrideFor(c.OpenRuntime, config.TierEveryday); v != "" {
		t.Fatalf("override not cleared: %q", v)
	}
	_, err = s.UpdateConfig(context.Background(), &proto.UpdateConfigRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if p.calls != 1 {
		t.Fatal("empty ordinary RPC reconfigured runner")
	}
}

func TestConfigApplyCounterexampleResetOrdering(t *testing.T) {
	s, p := counterexampleServer(t)
	store := &counterexampleDelete{Store: secrets.NewMemory(), entered: make(chan struct{}), release: make(chan struct{})}
	store.Set("fixture", "fake")
	s.cfgSvc = counterexampleConfig{s.cfgSvc, store}
	resetDone := make(chan *proto.ResetSetupResponse, 1)
	go func() {
		r, _ := s.ResetSetup(context.Background(), &proto.ResetSetupRequest{Confirmed: true})
		resetDone <- r
	}()
	<-store.entered // R is durable; reset has not applied live state.
	workDone := make(chan error, 1)
	go func() {
		s.reloadConfigFromDisk(context.Background())
		_, err := s.UpdateConfig(context.Background(), &proto.UpdateConfigRequest{OpenModel: "new-model"})
		workDone <- err
	}()
	// Before the fix this worker adopts R and publishes/applies R+1 while reset
	// is paused. After the fix both operations wait for reset's apply lock.
	var early bool
	select {
	case err := <-workDone:
		early = true
		if err != nil {
			t.Error(err)
		}
	case <-time.After(100 * time.Millisecond):
	}
	close(store.release)
	r := <-resetDone
	if r == nil || !r.Ok || !r.ConfigWritten || !r.LiveApplied {
		t.Fatalf("reset=%+v", r)
	}
	if !early {
		if err := <-workDone; err != nil {
			t.Fatal(err)
		}
	}
	disk, err := config.Load(s.cfgSvc.Path())
	if err != nil {
		t.Fatal(err)
	}
	live := s.cfgSvc.Get()
	if live.Snapshot().Revision != disk.Snapshot().Revision {
		t.Fatal("reset regressed live snapshot behind durable R+1")
	}
	if v, _ := live.Models.OverrideFor(live.OpenRuntime, config.TierEveryday); v != "new-model" {
		t.Fatalf("live model=%q", v)
	}
	if p.last.ResolvedOpenModel != "new-model" {
		t.Fatalf("last runtime model=%q", p.last.ResolvedOpenModel)
	}
}

type counterexampleRouter struct {
	open  agent.TurnRunner
	count int
}

func (r *counterexampleRouter) SetOpenProvider(p agent.TurnRunner) { r.open = p; r.count++ }
func (*counterexampleRouter) SetCloudProvider(agent.TurnRunner)    {}
func (*counterexampleRouter) Tiers() agent.Tiers                   { return agent.Tiers{} }

type counterexampleInference struct{}

func (*counterexampleInference) Name() string { return "fake" }
func (*counterexampleInference) Capabilities() inference.Capabilities {
	return inference.Capabilities{}
}
func (*counterexampleInference) Chat(_ context.Context, c inference.Call) (inference.Result, error) {
	return inference.Result{Blocks: []llm.Block{{Type: llm.BlockText, Text: c.Model}}}, nil
}
func (*counterexampleInference) StreamChat(context.Context, inference.Call) (inference.Stream, error) {
	return nil, nil
}
