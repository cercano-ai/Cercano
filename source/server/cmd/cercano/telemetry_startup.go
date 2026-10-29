package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"cercano/source/server/internal/telemetry"
)

// startAgentTelemetry transfers ownership to the returned shutdown function.
// The caller must stop usage producers before invoking shutdown.
func startAgentTelemetry(path, session string) (*telemetry.Collector, func() error, error) {
	return startAgentTelemetryWithDependencies(path, session, productionAccountingDefaults())
}

// The opener is per invocation, never a package-global test override. Real
// collectors retain transactional accounting and drain before store closure.
type productionAccountingDependencies struct {
	open func(string) (*telemetry.SQLiteStore, error)
}

func productionAccountingDefaults() productionAccountingDependencies {
	return productionAccountingDependencies{open: telemetry.NewSQLiteStore}
}

func composeProductionAccounting(a productionStartupAdmission, session string, d productionAccountingDependencies) (*telemetry.Collector, func() error, error) {
	// Accounting is passive: producers supply observations and the caller owns
	// their join before collector drainage. This does not activate other services
	// or bypass the canonical parity gate. Prepared callers need real preflight.
	if a.paths.config == "" || a.paths.conversations == "" || (!a.legacy && !a.generationPreflight) {
		return nil, nil, fmt.Errorf("accounting requires admitted production authority")
	}
	return startAgentTelemetryWithDependencies(filepath.Join(filepath.Dir(a.paths.config), "telemetry.db"), session, d)
}

func startAgentTelemetryWithDependencies(path, session string, d productionAccountingDependencies) (*telemetry.Collector, func() error, error) {
	if d.open == nil {
		return nil, nil, fmt.Errorf("accounting store opener required")
	}
	store, err := d.open(path)
	if err != nil {
		return nil, nil, err
	}
	if store == nil {
		return nil, nil, fmt.Errorf("accounting opener returned nil store")
	}
	collector := telemetry.NewCollector(store, 256)
	collector.SetSessionID(session)
	if err := collector.EnableAccounting(telemetry.AccountingOptions{}); err != nil {
		collector.Close()
		return nil, nil, errors.Join(err, store.Close())
	}
	var once sync.Once
	var closeErr error
	return collector, func() error { once.Do(func() { collector.Close(); closeErr = store.Close() }); return closeErr }, nil
}
