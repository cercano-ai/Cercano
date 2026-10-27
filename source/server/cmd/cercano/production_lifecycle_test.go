package main

import (
	"cercano/source/server/internal/catalog"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/enterprise"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/ollamacatalog"
	"cercano/source/server/pkg/config"
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
)

type startupStoreFake struct {
	conversation.Store
	closed int
}

func (s *startupStoreFake) Close() error { s.closed++; return nil }

func TestProductionListenerFailureClosesOwnedStore(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	dir := t.TempDir()
	store := &startupStoreFake{}
	d, runtime, _ := runtimeFixture(t)
	a := productionStartupAdmission{legacy: true, paths: productionAuthorityPaths{config: filepath.Join(dir, "config.yaml"), conversations: filepath.Join(dir, "history.db")}}
	providerOpens := 0
	deps := productionLifecycleDependencies{
		openProvider: func(config.Config) inference.Provider { providerOpens++; return &compositionCloudProvider{} },
		enterprise:   productionEnterpriseDependencies{options: &enterprise.HostOptions{Directory: filepath.Join(dir, "enterprise"), OpenStore: func() (enterprise.CredentialStore, error) { t.Fatal("native opener"); return nil, nil }}},
		openStore:    func(string) (conversation.Store, error) { return store, nil },
		listen:       func(string, string) (net.Listener, error) { return nil, errors.New("injected listener failure") },
		reap:         func() int { return 0 },
	}
	_, cleanup, err := startGRPCServerWithLifecycle(config.Defaults(), "fixture", nil, productionCredentialDependencies{}, a, d, productionCatalogDefaults(), productionAccountingDefaults(), deps)
	if providerOpens != 1 {
		t.Fatalf("open provider calls = %d", providerOpens)
	}
	if err == nil || cleanup != nil {
		t.Fatal("expected startup failure")
	}
	if store.closed != 1 || runtime.closed != 1 {
		t.Fatalf("closes store=%d runtime=%d", store.closed, runtime.closed)
	}
}

// The listener is never bound to a socket; failed composition owns it until
// serving starts. Its counter intentionally detects repeated Close calls.
type startupListenerFake struct {
	net.Listener
	closes int
}

func (l *startupListenerFake) Close() error { l.closes++; return nil }

func TestProductionFailureStages(t *testing.T) {
	for _, stage := range []string{"store", "catalog-manager", "catalog-registry"} {
		t.Run(stage, func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
			a := serviceAdmission(t)
			store := &startupStoreFake{}
			d, runtime, _ := runtimeFixture(t)
			listener := &startupListenerFake{}
			catalogStops := 0
			var catalogCtx context.Context
			catalogs := productionCatalogDependencies{
				newManager: func(string) *ollamacatalog.Manager {
					if stage == "catalog-manager" {
						return nil
					}
					return &ollamacatalog.Manager{}
				},
				load:     func(*ollamacatalog.Manager) error { return nil },
				registry: func(*ollamacatalog.Manager) *catalog.Registry { return nil },
				start:    func(_ *ollamacatalog.Manager, ctx context.Context) { catalogCtx = ctx },
				stop:     func(*ollamacatalog.Manager) { catalogStops++ },
			}
			providerOpens := 0
			deps := productionLifecycleDependencies{
				openProvider: func(config.Config) inference.Provider { providerOpens++; return &compositionCloudProvider{} },
				enterprise:   productionEnterpriseDependencies{options: &enterprise.HostOptions{Directory: filepath.Join(filepath.Dir(a.paths.config), "enterprise"), OpenStore: func() (enterprise.CredentialStore, error) { t.Fatal("credential opener"); return nil, nil }}},
				openStore: func(path string) (conversation.Store, error) {
					if path != a.paths.conversations {
						t.Fatal("store authority changed")
					}
					if stage == "store" {
						return nil, errors.New("store failure")
					}
					return store, nil
				},
				listen: func(string, string) (net.Listener, error) { return listener, nil }, reap: func() int { return 0 },
			}
			_, cleanup, err := startGRPCServerWithLifecycle(config.Defaults(), "fixture", nil, productionCredentialDependencies{}, a, d, catalogs, productionAccountingDefaults(), deps)
			if providerOpens != 1 {
				t.Fatalf("open provider calls = %d", providerOpens)
			}
			if err == nil || cleanup != nil {
				t.Fatal("expected failed composition")
			}
			want := 1
			if stage == "store" {
				want = 0
			}
			if runtime.closed != 1 || store.closed != want || listener.closes != want {
				t.Fatalf("runtime=%d store=%d listener=%d", runtime.closed, store.closed, listener.closes)
			}
			if stage == "catalog-registry" && (catalogStops != 1 || catalogCtx.Err() == nil) {
				t.Fatal("catalog not canceled exactly once")
			}
		})
	}
}

// This marker makes the fake remote attachment shared. A Stop call still
// panics through its nil embedded Manager, catching accidental remote stops.
func (f *compositionRuntimeFake) HoldEpoch() uint64 { return 1 }

func (f *compositionRuntimeFake) Inventory(context.Context) ([]localruntime.ModelRecord, error) {
	return nil, nil
}

func (f *compositionRuntimeFake) Instances(context.Context) ([]localruntime.InstanceRecord, error) {
	return nil, nil
}

func (f *compositionRuntimeFake) RegisterObserver(localruntime.Observer) {}
