package main

import (
	"cercano/source/server/internal/usage"
	"cercano/source/server/pkg/config"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"cercano/source/server/internal/catalog"
	"cercano/source/server/internal/ollamacatalog"
	"cercano/source/server/internal/telemetry"
)

func serviceAdmission(t *testing.T) productionStartupAdmission {
	t.Helper()
	dir := t.TempDir()
	return productionStartupAdmission{legacy: true, paths: productionAuthorityPaths{config: filepath.Join(dir, "config.yaml"), conversations: filepath.Join(dir, "conversations.db")}}
}

func TestProductionCatalogAccountingRefuseBeforeDependencies(t *testing.T) {
	a := serviceAdmission(t)
	a.legacy = false
	called := false
	catalogs := productionCatalogDefaults()
	catalogs.newManager = func(string) *ollamacatalog.Manager { called = true; return nil }
	accounting := productionAccountingDependencies{open: func(string) (*telemetry.SQLiteStore, error) { called = true; return nil, nil }}
	for _, admission := range []productionStartupAdmission{a, {}} {
		if close, err := composeProductionCatalog(nil, admission, catalogs); err == nil || close != nil {
			t.Fatal("catalog admitted unowned producer")
		}
		if c, close, err := composeProductionAccounting(admission, "fixture", accounting); err == nil || c != nil || close != nil {
			t.Fatal("accounting admitted unowned producer")
		}
	}
	if _, close, err := startGRPCServerWithServices(config.Config{}, "", nil, productionCredentialDependencies{}, a, productionRuntimeDependencies{}, catalogs, accounting); err == nil || close != nil {
		t.Fatal("startup admitted unowned producers")
	}
	if close, err := composeProductionCatalog(nil, serviceAdmission(t), productionCatalogDependencies{newManager: catalogs.newManager}); err == nil || close != nil {
		t.Fatal("incomplete dependencies admitted")
	}
	if called {
		t.Fatal("rejected policy reached dependency")
	}
}

func TestProductionCatalogOwnedTeardown(t *testing.T) {
	type host struct {
		close   func()
		ctx     context.Context
		stops   int
		manager *ollamacatalog.Manager
	}
	makeHost := func() *host {
		h := &host{}
		a := serviceAdmission(t)
		s := mcpCompositionServer(t)
		var order []string
		d := productionCatalogDependencies{
			newManager: func(path string) *ollamacatalog.Manager {
				if path != filepath.Join(filepath.Dir(a.paths.config), "catalog-cache.json") {
					t.Fatal(path)
				}
				order = append(order, "new")
				h.manager = ollamacatalog.NewManager(&ollamacatalog.Fetcher{}, path)
				return h.manager
			},
			load: func(m *ollamacatalog.Manager) error {
				if m != h.manager {
					t.Fatal("wrong manager")
				}
				order = append(order, "load")
				return errors.New("fake corrupt cache")
			},
			start: func(m *ollamacatalog.Manager, ctx context.Context) {
				if m != h.manager {
					t.Fatal("wrong manager")
				}
				h.ctx = ctx
				order = append(order, "start")
			},
			registry: func(m *ollamacatalog.Manager) *catalog.Registry {
				if m != h.manager {
					t.Fatal("wrong manager")
				}
				order = append(order, "registry")
				return catalog.NewRegistry()
			},
			stop: func(m *ollamacatalog.Manager) {
				if m != h.manager {
					t.Fatal("wrong manager")
				}
				h.stops++
			},
		}
		var err error
		h.close, err = composeProductionCatalog(s, a, d)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(h.close)
		if !reflect.DeepEqual(order, []string{"new", "load", "start", "registry"}) {
			t.Fatal(order)
		}
		return h
	}
	a, b := makeHost(), makeHost()
	a.close()
	a.close()
	if a.stops != 1 || a.ctx.Err() == nil || b.stops != 0 || b.ctx.Err() != nil {
		t.Fatal("cleanup crossed instance ownership")
	}
	b.close()
	if b.stops != 1 {
		t.Fatal("B not closed")
	}
}

func TestProductionAccountingOwnedDrain(t *testing.T) {
	a := serviceAdmission(t)
	path := filepath.Join(filepath.Dir(a.paths.config), "telemetry.db")
	opens := 0
	d := productionAccountingDependencies{open: func(got string) (*telemetry.SQLiteStore, error) {
		if got != path {
			t.Fatalf("authority path %s", got)
		}
		opens++
		return telemetry.NewSQLiteStore(got)
	}}
	first, closeFirst, err := composeProductionAccounting(a, "A", d)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFirst()
	second, closeSecond, err := composeProductionAccounting(a, "B", d)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSecond()
	first.Emit(telemetry.NewEvent("A", "fake"))
	if !first.EmitAttempt(usage.AttemptObservation{ID: "A-attempt", Revision: 1, StartedAt: time.Now(), Outcome: usage.Started}) {
		t.Fatal("attempt rejected")
	}
	closeFirst()
	closeFirst()
	second.Emit(telemetry.NewEvent("B-after-A", "fake"))
	if !second.EmitAttempt(usage.AttemptObservation{ID: "B-attempt", Revision: 1, StartedAt: time.Now(), Outcome: usage.Started}) {
		t.Fatal("sibling attempt rejected")
	}
	closeSecond()
	store, err := telemetry.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, id := range []string{"A-attempt", "B-attempt"} {
		if _, err := store.AccountingAttempt(context.Background(), id); err != nil {
			t.Fatalf("undrained %s: %v", id, err)
		}
	}
	stats, err := store.GetStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if opens != 2 || stats.TotalRequests != 2 {
		t.Fatalf("opens=%d persisted=%d", opens, stats.TotalRequests)
	}
}
