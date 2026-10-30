//go:build developmentfixture && cercano_development_acceptance

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"cercano/source/server/internal/enterprise"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/localruntime"
	mcphost "cercano/source/server/internal/mcp_host"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/watchdog"
	"cercano/source/server/pkg/config"
)

// Offline backend substitution only: use the ordinary entrypoint and its full
// service graph, rather than the old prepared acceptance graph.
func startOrdinaryGenerationServer(cfg config.Config, o developmentHostOptions, a productionStartupAdmission) (string, *productionShutdown, error) {
	barrier := os.Getenv("CERCANO_FIXTURE_BARRIER")
	if err := developmentPrivatePath(barrier, true); err != nil {
		return "", nil, err
	}
	fixture := filepath.Join(barrier, "mcp-fixture")
	if err := developmentPrivatePath(fixture, false); err != nil {
		return "", nil, err
	}
	mc, err := mcphost.LoadConfig(o.State)
	if err != nil {
		return "", nil, err
	}
	entry, ok := mc.Servers["fixture"]
	if !ok || len(mc.Servers) != 1 || entry.Command != fixture || len(entry.Args) != 0 || len(entry.Env) != 1 || entry.Env["CERCANO_DISABLE_CREDENTIALS"] != "1" {
		return "", nil, fmt.Errorf("ordinary acceptance requires fixed isolated MCP fixture")
	}
	d := developmentProductionDefaults()
	old, existed := os.LookupEnv("CERCANO_DISABLE_CREDENTIALS")
	defer func() {
		if existed {
			_ = os.Setenv("CERCANO_DISABLE_CREDENTIALS", old)
		} else {
			_ = os.Unsetenv("CERCANO_DISABLE_CREDENTIALS")
		}
	}()
	_ = os.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	runtimes := productionRuntimeDefaults()
	runtimes.generation = d.connectRuntime
	deps := productionLifecycleDefaults()
	deps.mcp = d.mcp
	deps.watchdog = server.WatchdogDependencies{
		OpenAudit: func(string) (watchdog.AuditLogger, error) {
			fixtureRecord("watchdog-open", inference.Call{})
			return ordinaryFixtureAudit{}, nil
		},
		OneShot: func(context.Context, string) (string, error) {
			fixtureRecord("watchdog-oneshot", inference.Call{})
			return "VIOLATION: no\n", nil
		},
	}
	deps.cloudBuilder = d.cloudBuilder
	deps.openProvider = func(c config.Config) inference.Provider {
		p, err := d.newOpenProvider(localruntime.NewManager(), c)
		if err != nil {
			panic(err)
		}
		return p
	}
	deps.enterprise = productionEnterpriseDependencies{options: &enterprise.HostOptions{Directory: filepath.Join(o.State, "enterprise"), Manager: enterprise.Options{HTTPClient: &http.Client{Transport: ordinaryFixtureIssuer{}}}, OpenStore: func() (enterprise.CredentialStore, error) { return secrets.NewMemory(), nil }}}
	return startGRPCServerWithLifecycle(cfg, o.Listen, nil, productionCredentialDependencies{openSecrets: d.openSecrets}, a, runtimes, productionCatalogDefaults(), productionAccountingDefaults(), deps)
}

// Offline replacements retain enabled services without dialing or native stores.
type ordinaryFixtureAudit struct{}

func (ordinaryFixtureAudit) Record(_ context.Context, e watchdog.AuditEvent) error {
	fixtureRecord("watchdog-audit", inference.Call{ConversationID: e.ConversationID, Model: e.Check})
	return nil
}
func (ordinaryFixtureAudit) Close() error { return nil }

type ordinaryFixtureIssuer struct{}

func (ordinaryFixtureIssuer) RoundTrip(r *http.Request) (*http.Response, error) {
	fixtureRecord("issuer", inference.Call{})
	return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("offline issuer")), Request: r}, nil
}
