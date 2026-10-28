package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"cercano/source/server/internal/catalog"
	"cercano/source/server/internal/deepinfracatalog"
	"cercano/source/server/internal/modelcatalog"
	"cercano/source/server/internal/ollamacatalog"
	"cercano/source/server/internal/server"
)

// All constructors and lifecycle operations are invocation-local. The concrete
// manager is retained because provider/model resolution consumes that API.
// Prepared hosts must not start these uncoordinated shared-cache producers.
type productionCatalogDependencies struct {
	newManager func(string) *ollamacatalog.Manager
	registry   func(*ollamacatalog.Manager) *catalog.Registry
	load       func(*ollamacatalog.Manager) error
	start      func(*ollamacatalog.Manager, context.Context)
	stop       func(*ollamacatalog.Manager)
}

func productionCatalogDefaults() productionCatalogDependencies {
	return productionCatalogDependencies{
		newManager: func(path string) *ollamacatalog.Manager {
			return ollamacatalog.NewManager(&ollamacatalog.Fetcher{}, path)
		},
		registry: func(m *ollamacatalog.Manager) *catalog.Registry {
			r := catalog.NewRegistry()
			r.Register(modelcatalog.NewBackend(&modelcatalog.Client{}))
			r.Register(ollamacatalog.NewBackend(m))
			r.Register(deepinfracatalog.New())
			return r
		},
		load:  (*ollamacatalog.Manager).LoadCache,
		start: (*ollamacatalog.Manager).Start,
		stop:  (*ollamacatalog.Manager).Stop,
	}
}

func (d productionCatalogDependencies) validate() error {
	if d.newManager == nil || d.registry == nil || d.load == nil || d.start == nil || d.stop == nil {
		return fmt.Errorf("complete production catalog dependencies required")
	}
	return nil
}

func composeProductionCatalog(s *server.Server, a productionStartupAdmission, d productionCatalogDependencies) (func(), error) {
	// This gate deliberately precedes factories, cache reads and goroutines.
	if err := a.validateActivation(); err != nil {
		return nil, err
	}
	if err := d.validate(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("catalog server required")
	}
	path := filepath.Join(filepath.Dir(a.paths.config), "catalog-cache.json")
	m := d.newManager(path)
	if m == nil {
		return nil, fmt.Errorf("catalog factory returned nil manager")
	}
	if err := d.load(m); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Could not load online-catalog cache at %s: %v (starting fresh)\n", path, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.start(m, ctx)
	var once sync.Once
	close := func() { once.Do(func() { cancel(); d.stop(m) }) }
	s.SetCatalogManager(m)
	r := d.registry(m)
	if r == nil {
		close()
		return nil, fmt.Errorf("catalog factory returned nil registry")
	}
	s.SetCatalogRegistry(r)
	// Stop joins this invocation's refresh and warming callbacks.
	// Prepared activation remains disabled.
	return close, nil
}
