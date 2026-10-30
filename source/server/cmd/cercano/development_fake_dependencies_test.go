package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/localruntime"
	mcphost "cercano/source/server/internal/mcp_host"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/telemetry"
	"cercano/source/server/internal/watchdog"
	"cercano/source/server/pkg/config"
)

// Reachability audit: prepared production construction installs idle scheduling
// producers; it does not enqueue work. Compactor construction may query runtime
// capacity through the in-memory manager below, without preparing an instance.
// Routing's cloud factory and embedder are lazy; Read/ListTools never enter them.
// InstallCapabilities registers callbacks, not executions. MCP uses a real
// invocation-local manager but these graph-only fixtures suppress its Start;
// stdio startup is qualified separately. composeProductionServices skips InitWatchdog. ConfigurePreparedWorker only constructs a pool; these snapshots
// set worker_idle_timeout_seconds: -1 to disable even its idle reaper. Never call
// a turn/worker/task capability here. This is bounded graph/readiness acceptance,
// not inference, worker execution, or full startup acceptance.
// No production defaults, native stores, runtime providers or global replacements.
type developmentBoundaryCounts struct{ opener, runtime, open, cloud int }

func fullDevelopmentFakes(t *testing.T, overrides developmentProductionDependencies) (developmentProductionDependencies, *developmentBoundaryCounts) {
	t.Helper()
	counts := &developmentBoundaryCounts{}
	memory := secrets.NewMemory()
	d := overrides
	if d.accounting.open == nil {
		d.accounting.open = func(string) (*telemetry.SQLiteStore, error) { return telemetry.NewSQLiteStore(":memory:") }
	}
	if d.openSecrets == nil {
		d.openSecrets = func() (secrets.Store, error) { return memory, nil }
	}
	if d.connectRuntime == nil {
		d.connectRuntime = func(context.Context, config.Config, string, string) (localruntime.Manager, func() error, error) {
			return localruntime.NewManager(), func() error { return nil }, nil
		}
	}
	if d.newOpenProvider == nil {
		d.newOpenProvider = func(localruntime.Manager, config.Config) (inference.Provider, error) {
			return &developmentSyntheticProvider{}, nil
		}
	}
	if d.cloudBuilder == nil {
		d.cloudBuilder = func(config.CloudProfile, string, ...cloudfactory.Options) (inference.Provider, error) {
			return &developmentSyntheticProvider{}, nil
		}
	}
	if d.watchdog.OpenAudit == nil {
		d.watchdog.OpenAudit = func(string) (watchdog.AuditLogger, error) { panic("unexpected watchdog audit construction") }
	}
	if d.watchdog.OneShot == nil {
		d.watchdog.OneShot = func(context.Context, string) (string, error) { panic("unexpected watchdog inference") }
	}
	if d.mcp.newManager == nil {
		d.mcp = productionMCPDependencies{
			newManager: func(reg *agenttools.Registry, dir string, wait time.Duration) *mcphost.Manager {
				return mcphost.New(reg, dir, wait)
			},
			start: func(*mcphost.Manager, context.Context) {},
		}
	}
	opener, runtime, open, cloud := d.openSecrets, d.connectRuntime, d.newOpenProvider, d.cloudBuilder
	d.openSecrets = func() (secrets.Store, error) { counts.opener++; return opener() }
	d.connectRuntime = func(ctx context.Context, cfg config.Config, state, host string) (localruntime.Manager, func() error, error) {
		counts.runtime++
		return runtime(ctx, cfg, state, host)
	}
	d.newOpenProvider = func(m localruntime.Manager, cfg config.Config) (inference.Provider, error) {
		counts.open++
		return open(m, cfg)
	}
	d.cloudBuilder = func(p config.CloudProfile, key string, opts ...cloudfactory.Options) (inference.Provider, error) {
		counts.cloud++
		return cloud(p, key, opts...)
	}
	return d, counts
}

func enabledDevelopmentFakes(t *testing.T, overrides developmentProductionDependencies, want ...developmentBoundaryCounts) developmentProductionDependencies {
	t.Helper()
	d, counts := fullDevelopmentFakes(t, overrides)
	if len(want) > 0 {
		t.Cleanup(func() {
			if *counts != want[0] {
				t.Errorf("boundaries=%+v want=%+v", *counts, want[0])
			}
		})
	}
	// Policy is enabled only after every external construction seam is fake.
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	return d
}

// Keep policy rejection independent of graph acceptance, including fail-closed
// empty and malformed environment values. Every early boundary is counted.
func TestDevelopmentProductionCredentialPolicyBeforeDependencies(t *testing.T) {
	for _, value := range []string{"1", "", "malformed"} {
		t.Run("policy="+value, func(t *testing.T) {
			o, auth := developmentProductionFixture(t)
			d, counts := fullDevelopmentFakes(t, developmentProductionDependencies{})
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", value)
			srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, d)
			if cleanup != nil {
				defer cleanup.Wait(context.Background())
			}
			if !errors.Is(err, secrets.ErrDisabled) || srv != nil || *counts != (developmentBoundaryCounts{}) {
				t.Fatalf("policy=%q server=%v error=%v boundaries=%+v", value, srv != nil, err, counts)
			}
		})
	}
}
