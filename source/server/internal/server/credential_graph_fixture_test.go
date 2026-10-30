package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

// This fixture exercises graph publication only: no capability installation,
// worker selection, turn execution, native credential backend or transport.
type graphFixtureProvider struct{}

func (*graphFixtureProvider) Name() string                         { return "graph-fixture" }
func (*graphFixtureProvider) Capabilities() inference.Capabilities { return inference.Capabilities{} }
func (*graphFixtureProvider) Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	panic("graph fixture must not execute inference")
}
func (*graphFixtureProvider) StreamChat(context.Context, llm.ChatRequest) (llm.StreamReader, error) {
	panic("graph fixture must not execute inference")
}

type graphFixtureSecrets struct {
	secrets.Store
	reads atomic.Int64
}

func (s *graphFixtureSecrets) Get(name string) (string, error) {
	s.reads.Add(1)
	return s.Store.Get(name)
}

// Install every dependency before relaxing admission, and first prove that the
// real production rebuild refuses without even reading the memory store.
func allowMemoryGraphFixture(t *testing.T, s *Server) {
	t.Helper()
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	store := &graphFixtureSecrets{Store: secrets.NewMemory()}
	for _, p := range s.cfgSvc.Get().CloudProfiles {
		// Subscription/Bedrock routes may resolve credentials before the fake
		// builder is called. This helper only admits direct messages fixtures.
		if p.Flavor != "messages" || (p.Route != "" && p.Route != "direct") {
			t.Fatalf("unsupported graph fixture route: %s/%s", p.Flavor, p.Route)
		}
		if err := store.Store.Set(p.Name, "fixture-key"); err != nil {
			t.Fatal(err)
		}
	}
	s.cfgSvc.SetSecrets(store)
	var builds atomic.Int64
	s.SetCloudProviderBuilder(func(config.CloudProfile, string, ...cloudfactory.Options) (inference.Provider, error) {
		builds.Add(1)
		return &graphFixtureProvider{}, nil
	})
	if err := s.rebuildCloud(); !errors.Is(err, secrets.ErrDisabled) {
		t.Fatalf("disabled rebuild: %v", err)
	}
	if store.reads.Load() != 0 || builds.Load() != 0 {
		t.Fatalf("disabled dependencies: reads=%d builds=%d", store.reads.Load(), builds.Load())
	}
	credentialtest.Enable(t)
	t.Cleanup(func() {
		if builds.Load() == 0 {
			t.Error("enabled fixture never built a provider")
		}
	})
}
