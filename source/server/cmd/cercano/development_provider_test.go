package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

type developmentSyntheticProvider struct{}

func (*developmentSyntheticProvider) Name() string { return "development-synthetic" }
func (*developmentSyntheticProvider) Capabilities() inference.Capabilities {
	return inference.Capabilities{}
}
func (*developmentSyntheticProvider) Chat(context.Context, inference.Call) (inference.Result, error) {
	return inference.Result{}, errors.New("unexpected inference")
}
func (*developmentSyntheticProvider) StreamChat(context.Context, inference.Call) (inference.Stream, error) {
	return nil, errors.New("unexpected inference")
}

func TestDevelopmentProductionProviderConstructor(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "synthetic", true: "error"}[fail], func(t *testing.T) {
			o, auth := developmentProductionFixture(t)
			fake := &developmentSyntheticProvider{}
			calls, closed := 0, 0
			sentinel := errors.New("constructor refused")
			srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{
				openSecrets: func() (secrets.Store, error) { return secrets.NewMemory(), nil },
				connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func(), error) {
					return localruntime.NewManager(), func() { closed++ }, nil
				},
				newOpenProvider: func(localruntime.Manager, config.Config) (inference.Provider, error) {
					calls++
					if fail {
						return nil, sentinel
					}
					return fake, nil
				},
			})
			if calls != 1 {
				t.Fatalf("constructor calls %d", calls)
			}
			if fail {
				if !errors.Is(err, sentinel) || cleanup != nil || closed != 1 {
					t.Fatalf("error=%v cleanup=%v closed=%d", err, cleanup != nil, closed)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			defer srv.Shutdown()
			if srv.DispatchCandidates().Open != fake {
				t.Fatal("synthetic provider did not reach production dispatch graph")
			}
		})
	}
}

func TestDevelopmentProductionCloudOnlyWithoutRuntime(t *testing.T) {
	o, auth := developmentProductionFixture(t)
	// An intentionally unusable local runtime must be irrelevant to cloud-only.
	snapshot := "open_runtime: unavailable\nlocus_mode: cloud_only\nactive_cloud_profile: cloud\ncloud_profiles:\n  - name: cloud\n    provider: openai\n    flavor: chat_completions\n    model: synthetic-model\n    base_url: http://127.0.0.1:1/v1\n"
	if err := os.WriteFile(filepath.Join(o.State, "config.yaml"), []byte(snapshot), 0600); err != nil {
		t.Fatal(err)
	}
	memory := secrets.NewMemory()
	if err := memory.Set("cloud", "synthetic-key"); err != nil {
		t.Fatal(err)
	}
	srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{
		openSecrets: func() (secrets.Store, error) { return memory, nil },
		connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func(), error) {
			t.Fatal("cloud-only entered runtime connector")
			return nil, nil, nil
		},
		newOpenProvider: func(localruntime.Manager, config.Config) (inference.Provider, error) {
			t.Fatal("cloud-only constructed local provider")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer srv.Shutdown()
	if srv.DispatchCandidates().Open != nil {
		t.Fatal("cloud-only installed an open fallback")
	}
}

func TestDevelopmentProductionLocalRequiresExplicitAttachment(t *testing.T) {
	o, _ := developmentProductionFixture(t)
	_, _, err := connectDevelopmentRuntime(context.Background(), config.Defaults(), o.State, o.Host)
	if err == nil {
		t.Fatal("local attachment discovered an ambient runtime without a state resource")
	}
}

func TestDevelopmentProductionCloudConstructionFailsClosed(t *testing.T) {
	o, auth := developmentProductionFixture(t)
	snapshot := "locus_mode: cloud_only\nactive_cloud_profile: cloud\ncloud_profiles:\n  - name: cloud\n    provider: invalid\n    flavor: invalid\n    model: test-model\n"
	if err := os.WriteFile(filepath.Join(o.State, "config.yaml"), []byte(snapshot), 0600); err != nil {
		t.Fatal(err)
	}
	memory := secrets.NewMemory()
	if err := memory.Set("cloud", "synthetic-key"); err != nil {
		t.Fatal(err)
	}
	srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{openSecrets: func() (secrets.Store, error) { return memory, nil }})
	if err == nil || srv != nil || cleanup != nil {
		t.Fatalf("invalid cloud route survived: server=%v cleanup=%v error=%v", srv != nil, cleanup != nil, err)
	}
}
