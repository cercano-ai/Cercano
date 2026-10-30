package main

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"

	"cercano/source/server/internal/catalog"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/enterprise"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/ollamacatalog"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/watchdog"
	"cercano/source/server/pkg/config"
)

// Real ordinary entrypoint and all ordinary services. Only external backends
// are fake; enabled watchdog and memory credentials remain wired.
func TestOrdinaryGenerationStartup(t *testing.T) {
	startOrdinaryGenerationTest(t)
}

func startOrdinaryGenerationTest(t *testing.T) *server.Server {
	t.Helper()
	o, auth := developmentProductionFixture(t)
	t.Setenv("HOME", o.State)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(o.State, "xdg"))
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	a := o.startupAdmission
	a.generation = &ordinaryGenerationBinding{options: o, auth: auth}
	cfg, err := config.Load(a.paths.config)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Agent.ShutdownOnLastClient = true
	cfg.Watchdog.Enabled = true
	cfg.Watchdog.Audit.Enabled = true
	cfg.Watchdog.Audit.Path = filepath.Join(o.State, "audit.db")
	if err := config.Save(cfg, a.paths.config); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load(a.paths.config)
	if err != nil {
		t.Fatal(err)
	}
	runtimes, manager, _ := runtimeFixture(t)
	runtimes.generation = func(_ context.Context, _ config.Config, state, host string) (localruntime.Manager, func() error, error) {
		if state != o.State || host != o.Host {
			t.Fatal("runtime authority mismatch")
		}
		return manager, func() error { return nil }, nil
	}
	catalogs := productionCatalogDependencies{
		newManager: func(string) *ollamacatalog.Manager { return &ollamacatalog.Manager{} }, load: func(*ollamacatalog.Manager) error { return nil },
		registry: func(*ollamacatalog.Manager) *catalog.Registry { return catalog.NewRegistry() }, start: func(*ollamacatalog.Manager, context.Context) {}, stop: func(*ollamacatalog.Manager) {},
	}
	deps := productionLifecycleDefaults()
	deps.openStore = func(string) (conversation.Store, error) { t.Fatal("generation substituted store"); return nil, nil }
	deps.reap = func() int { t.Fatal("generation invoked legacy process-wide orphan reap"); return 0 }
	deps.listen = func(string, string) (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:0") }
	deps.runtimeHTTPClient = &http.Client{Transport: startupTransportTrap{}}
	deps.newOllama = func(string) productionOllamaEngine { return &startupEngineTrap{} }
	deps.openProvider = func(config.Config) inference.Provider { return &startupProviderTrap{} }
	deps.routingLogPath = filepath.Join(o.State, "routing.jsonl")
	deps.failureLogPath = filepath.Join(o.State, "failures.jsonl")
	deps.enterprise = productionEnterpriseDependencies{options: &enterprise.HostOptions{Directory: filepath.Join(o.State, "enterprise"), Manager: enterprise.Options{HTTPClient: &http.Client{Transport: startupTransportTrap{}}}, OpenStore: func() (enterprise.CredentialStore, error) { panic("native credentials") }}}
	audit := &startupAudit{}
	audits := 0
	deps.watchdog = server.WatchdogDependencies{OpenAudit: func(string) (watchdog.AuditLogger, error) { audits++; return audit, nil }, OneShot: func(context.Context, string) (string, error) { panic("watchdog inference") }}
	wired := false
	var srv *server.Server
	deps.afterWiring = func(s *server.Server) {
		wired = true
		srv = s
		if s.GenerationStore() == nil || reflect.ValueOf(s).Elem().FieldByName("workerRunner").IsNil() {
			t.Fatal("ordinary generation installer absent")
		}
	}
	credentials := productionCredentialDependencies{openSecrets: func() (secrets.Store, error) { return secrets.NewMemory(), nil }}
	address, cleanup, err := startGRPCServerWithLifecycle(cfg, "127.0.0.1:0", nil, credentials, a, runtimes, catalogs, productionAccountingDefaults(), deps)
	if cleanup != nil {
		t.Cleanup(func() {
			cleanup.Request()
			<-cleanup.done
			if err := cleanup.Status().Err; err != nil {
				t.Error(err)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	if address == "" || !wired || audits != 1 {
		t.Fatalf("ordinary services missing: %s %v %d", address, wired, audits)
	}
	return srv
}
