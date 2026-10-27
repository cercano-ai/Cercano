package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/engine"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/watchdog"
	"cercano/source/server/pkg/config"
)

type compositionCloudProvider struct{ developmentSyntheticProvider }

func (*compositionCloudProvider) Chat(context.Context, inference.Call) (inference.Result, error) {
	return inference.Result{Model: "fixture"}, nil
}

// Exercise actual ordinary and prepared server provider services, not name lists.
// Prepared request admission and background producers remain disabled.
func TestProductionProviderPairedRebuild(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("HOME", t.TempDir())
	o, auth := developmentProductionFixture(t)
	body := []byte("open_runtime: llama_server\nlocus_mode: open_only\nactive_cloud_profile: fixture\ncloud_profiles:\n  - name: fixture\n    provider: anthropic\n    flavor: messages\n    route: subscription\n    model: fixture\n")
	body = append(body, []byte("watchdog:\n  enabled: true\n  audit:\n    enabled: true\n")...)
	path := filepath.Join(o.State, "config.yaml")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	calls := [2]int{}
	builder := func(i int) func(config.CloudProfile, string, ...cloudfactory.Options) (inference.Provider, error) {
		return func(p config.CloudProfile, _ string, _ ...cloudfactory.Options) (inference.Provider, error) {
			if p.Name != "fixture" {
				t.Fatalf("unexpected profile %q", p.Name)
			}
			calls[i]++
			return &compositionCloudProvider{}, nil
		}
	}
	routing := composeProductionRouting(productionRoutingDependencies{open: &compositionRunner{name: "open"}, classificationDisabled: true})
	ordinary := server.NewServer(agent.NewAgent(routing.router, routing.coordinator), routing.router, routing.coordinator, nil, engine.NewEngineRegistry())
	defer ordinary.Shutdown()
	ordinary.SetSecrets(secrets.NewMemory())
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ordinary.SetConfigPersistence(filepath.Join(t.TempDir(), "config.yaml"), cfg); err != nil {
		t.Fatal(err)
	}
	ordinary.SetCloudProviderBuilder(builder(0))
	prepared, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{
		cloudBuilder: builder(1),
		watchdog: server.WatchdogDependencies{OpenAudit: func(string) (watchdog.AuditLogger, error) {
			t.Fatal("prepared watchdog opened unowned audit")
			return nil, nil
		}, OneShot: func(context.Context, string) (string, error) { t.Fatal("prepared watchdog dispatched"); return "", nil }},
		openSecrets: func() (secrets.Store, error) { return secrets.NewMemory(), nil },
		connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func(), error) {
			return localruntime.NewManager(), func() {}, nil
		},
		newOpenProvider: func(localruntime.Manager, config.Config) (inference.Provider, error) {
			return &compositionCloudProvider{}, nil
		},
	})
	// Startup itself may refuse before publishing a prepared server. Do not
	// enable policy merely to let the subscription fixture reach its fake.
	if err != nil {
		if !errors.Is(err, secrets.ErrDisabled) {
			t.Fatal(err)
		}
		if cleanup != nil {
			cleanup()
		}
	} else {
		defer cleanup()
		defer prepared.Shutdown()
		if err := prepared.RebuildCloud(); !errors.Is(err, secrets.ErrDisabled) {
			t.Fatalf("prepared credential refusal: %v", err)
		}
		if prepared.CloudLLMProvider() != nil {
			t.Fatal("disabled prepared graph published provider")
		}
	}
	if err := ordinary.RebuildCloud(); !errors.Is(err, secrets.ErrDisabled) {
		t.Fatalf("ordinary credential refusal: %v", err)
	}
	if ordinary.CloudLLMProvider() != nil {
		t.Fatal("disabled ordinary graph published provider")
	}

	if calls != [2]int{} {
		t.Fatalf("disabled policy invoked builders: %v", calls)
	}
}

func TestProductionProviderActivationBeforeDependencies(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	called := false
	deps := productionLifecycleDependencies{
		cloudBuilder: func(config.CloudProfile, string, ...cloudfactory.Options) (inference.Provider, error) {
			called = true
			return nil, nil
		},
		watchdog: server.WatchdogDependencies{OpenAudit: func(string) (watchdog.AuditLogger, error) { called = true; return nil, nil }, OneShot: func(context.Context, string) (string, error) { called = true; return "", nil }},
	}
	p := fixtureStartupPolicy(t, false)
	paths, err := p.authorities.resolve()
	if err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := startGRPCServerWithLifecycle(config.Defaults(), "", nil, productionCredentialDependencies{}, productionStartupAdmission{paths: paths}, productionRuntimeDependencies{}, productionCatalogDependencies{}, productionAccountingDependencies{}, deps)
	if err == nil || !strings.Contains(err.Error(), "activation disabled") || cleanup != nil || called {
		t.Fatalf("admission failed: %v, called=%v", err, called)
	}
}
