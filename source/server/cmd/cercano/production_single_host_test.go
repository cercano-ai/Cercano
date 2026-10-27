package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/catalog"
	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/engine"
	"cercano/source/server/internal/enterprise"
	"cercano/source/server/internal/inference"
	mcphost "cercano/source/server/internal/mcp_host"
	"cercano/source/server/internal/modelpolicy"
	"cercano/source/server/internal/ollamacatalog"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/telemetry"
	"cercano/source/server/internal/watchdog"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Nil embedded interfaces deliberately panic before any backend entry. Only
// metadata is implemented: this fixture must never infer, embed, or spawn.
type startupEngineTrap struct{ engine.InferenceEngine }

func (*startupEngineTrap) Name() string { return "ollama" }
func (*startupEngineTrap) Embed(context.Context, string, string) ([]float64, error) {
	panic("startup attempted embedding")
}

type startupAudit struct{ closes atomic.Int32 }

func (*startupAudit) Record(context.Context, watchdog.AuditEvent) error {
	panic("unexpected watchdog audit event")
}
func (a *startupAudit) Close() error { a.closes.Add(1); return nil }

type startupProviderTrap struct{ inference.Provider }
type startupTransportTrap struct{}

func (startupTransportTrap) RoundTrip(*http.Request) (*http.Response, error) {
	panic("startup attempted external HTTP")
}

type countedStartupStore struct {
	conversation.Store
	conversation.TaskStore
	closes atomic.Int32
}

func (s *countedStartupStore) Close() error { s.closes.Add(1); return s.Store.Close() }

type countedStartupListener struct {
	*bufconn.Listener
	closes atomic.Int32
}

func (l *countedStartupListener) Close() error { l.closes.Add(1); return l.Listener.Close() }

// Sequential subtests own ONE full Agent/policy authority at a time. This is a
// startup-only, in_process check, not worker acceptance or model qualification.
func TestProductionSingleHostStartupShutdown(t *testing.T) {
	for _, failMCP := range []bool{false, true} {
		name := "serves"
		if failMCP {
			name = "late-mcp-failure"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
			restoreSentinel := modelpolicy.Install(modelpolicy.AuthorizeFunc(func(context.Context, modelpolicy.Attempt) error { panic("unexpected model authorization") }))
			defer restoreSentinel()
			t.Setenv("CERCANO_AUTOLAUNCHED", "0")
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
			a := productionStartupAdmission{legacy: true, paths: productionAuthorityPaths{config: filepath.Join(dir, "config.yaml"), conversations: filepath.Join(dir, "history.db")}}
			cfg := config.Config{OpenRuntime: "ollama", OllamaURL: "http://unreachable.invalid", ExecutionMode: "in_process", LocusMode: "open_only"}
			cfg.Watchdog.Enabled = true
			cfg.Watchdog.Audit.Enabled = true
			cfg.Watchdog.Audit.Path = filepath.Join(dir, "audit.db")
			if err := config.Save(cfg, a.paths.config); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "permissions.yaml"), []byte("mode: permissive\n"), 0600); err != nil {
				t.Fatal(err)
			}
			rawStore, err := conversation.Open(a.paths.conversations)
			if err != nil {
				t.Fatal(err)
			}
			store := &countedStartupStore{Store: rawStore, TaskStore: rawStore.(conversation.TaskStore)}
			listener := &countedStartupListener{Listener: bufconn.Listen(1 << 20)}
			runtimes, runtime, runtimeCalls := runtimeFixture(t)
			var catCtx, mcpCtx context.Context
			catDone, mcpDone := make(chan struct{}), make(chan struct{})
			var wired int
			var catStops, mcpStarts, mcpStops, providers, engines, reaps, audits, accountingOpens int
			catalogs := productionCatalogDependencies{
				newManager: func(path string) *ollamacatalog.Manager {
					if path != filepath.Join(dir, "catalog-cache.json") {
						t.Fatal(path)
					}
					return &ollamacatalog.Manager{}
				},
				load:     func(*ollamacatalog.Manager) error { return nil },
				registry: func(*ollamacatalog.Manager) *catalog.Registry { return catalog.NewRegistry() },
				start: func(_ *ollamacatalog.Manager, ctx context.Context) {
					catCtx = ctx
					go func() { <-ctx.Done(); close(catDone) }()
				},
				stop: func(*ollamacatalog.Manager) { catStops++; <-catDone },
			}
			audit := &startupAudit{}
			// A fake-only positive control demonstrates the store is usable without
			// enabling credential policy; admission must still refuse its opener.
			memory := secrets.NewMemory()
			if err := memory.Set("fixture", "not-a-real-key"); err != nil {
				t.Fatal(err)
			}
			if key, err := memory.Get("fixture"); err != nil || key != "not-a-real-key" {
				t.Fatal("memory control", err)
			}
			memoryOpens := 0
			memoryCredentials := productionCredentialDependencies{openSecrets: func() (secrets.Store, error) { memoryOpens++; return memory, nil }}
			credentials := productionCredentialDependencies{openSecrets: func() (secrets.Store, error) { panic("native credential opener reached") }}
			// Both writable and read-only admission must fail before even opening a backend.
			for _, opts := range []secrets.Options{{}, {ReadOnly: true}} {
				if _, err := memoryCredentials.open(opts); err == nil {
					t.Fatal("disabled memory admission succeeded")
				}
				if _, err := credentials.open(opts); err == nil {
					t.Fatal("disabled credential admission succeeded")
				}
			}
			if memoryOpens != 0 {
				t.Fatal("disabled policy entered memory opener")
			}
			deps := productionLifecycleDependencies{
				runtimeHTTPClient: &http.Client{Transport: startupTransportTrap{}},
				afterWiring: func(srv *server.Server) {
					wired++
					// Observe the actual startup instance without widening Server API for tests.
					// A nil runner cannot spawn worker subprocesses; no turn is requested.
					if !reflect.ValueOf(srv).Elem().FieldByName("workerRunner").IsNil() {
						t.Fatal("startup armed worker execution")
					}
				},
				newOllama:      func(string) productionOllamaEngine { engines++; return &startupEngineTrap{} },
				routingLogPath: filepath.Join(dir, "routing.jsonl"), failureLogPath: filepath.Join(dir, "failures.jsonl"),
				openProvider: func(config.Config) inference.Provider { providers++; return &startupProviderTrap{} },
				cloudBuilder: func(config.CloudProfile, string, ...cloudfactory.Options) (inference.Provider, error) {
					panic("cloud builder reached")
				},
				enterprise: productionEnterpriseDependencies{options: &enterprise.HostOptions{Directory: filepath.Join(dir, "enterprise"), Manager: enterprise.Options{HTTPClient: &http.Client{Transport: startupTransportTrap{}}}, OpenStore: func() (enterprise.CredentialStore, error) { panic("enterprise native opener reached") }}, openBrowser: func(context.Context, string) error { panic("browser launch reached") }},
				openStore: func(path string) (conversation.Store, error) {
					if path != a.paths.conversations {
						t.Fatal(path)
					}
					return store, nil
				},
				listen: func(network, address string) (net.Listener, error) {
					if network != "tcp" || address != "fixture" {
						t.Fatal(network, address)
					}
					return listener, nil
				},
				reap: func() int { reaps++; return 0 },
				watchdog: server.WatchdogDependencies{OpenAudit: func(path string) (watchdog.AuditLogger, error) {
					audits++
					if path != cfg.Watchdog.Audit.Path {
						t.Fatal(path)
					}
					return audit, nil
				}, OneShot: func(context.Context, string) (string, error) { panic("watchdog inference reached") }},
				mcp: productionMCPDependencies{
					newManager: func(reg *agenttools.Registry, path string, _ time.Duration) *mcphost.Manager {
						if path != dir || reg == nil {
							t.Fatal("MCP wiring")
						}
						if failMCP {
							return nil
						}
						return mcphost.New(reg, path, time.Second)
					},
					start: func(_ *mcphost.Manager, ctx context.Context) {
						mcpStarts++
						mcpCtx = ctx
						go func() { <-ctx.Done(); close(mcpDone) }()
					},
					stop: func(*mcphost.Manager) { mcpStops++; <-mcpDone },
				},
			}
			var telemetryStore *telemetry.SQLiteStore
			accounting := productionAccountingDependencies{open: func(path string) (*telemetry.SQLiteStore, error) {
				accountingOpens++
				if path != filepath.Join(dir, "telemetry.db") {
					t.Fatal(path)
				}
				var err error
				telemetryStore, err = telemetry.NewSQLiteStore(path)
				return telemetryStore, err
			}}
			addr, cleanup, err := startGRPCServerWithLifecycle(cfg, "fixture", nil, credentials, a, runtimes, catalogs, accounting, deps)
			if failMCP {
				if err == nil || err.Error() != "MCP factory returned nil manager" || cleanup != nil {
					t.Fatalf("expected late failure: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				defer cleanup()
				if addr != "bufconn" {
					t.Fatal(addr)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				conn, err := grpc.DialContext(ctx, "passthrough:///fixture", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }), grpc.WithBlock())
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				if modelpolicy.Managed(context.Background()) {
					t.Fatal("standalone host did not replace sentinel authority")
				}
				// bufconn is not a TCP loopback peer: retain enterprise control admission.
				if _, err := proto.NewEnterpriseClient(conn).GetStatus(ctx, &proto.EnterpriseControlRequest{}); status.Code(err) != codes.PermissionDenied {
					t.Fatalf("enterprise service: %v", err)
				}
				client := proto.NewAgentClient(conn)
				policy, err := client.GetCredentialPolicy(ctx, &proto.GetCredentialPolicyRequest{})
				if err != nil || !policy.GetCredentialsDisabled() {
					t.Fatalf("credential wiring: %v %v", policy, err)
				}
				got, err := client.GetConfig(ctx, &proto.GetConfigRequest{})
				if err != nil || got.GetOllamaUrl() != cfg.OllamaURL || !got.GetWatchdogEnabled() {
					t.Fatalf("config wiring: %v %v", got, err)
				}
				tools, err := client.ListTools(ctx, &proto.ListToolsRequest{})
				if err != nil || len(tools.GetTools()) == 0 {
					t.Fatalf("capabilities not wired: %v %v", tools, err)
				}
				if _, err := client.ListConversations(ctx, &proto.ListConversationsRequest{}); err != nil {
					t.Fatal(err)
				}
				var wg sync.WaitGroup
				for i := 0; i < 4; i++ {
					wg.Add(1)
					go func() { defer wg.Done(); cleanup() }()
				}
				wg.Wait()
				if _, err := client.GetConfig(ctx, &proto.GetConfigRequest{}); err == nil {
					t.Fatal("serving after shutdown")
				}
			}
			if (!failMCP && wired != 1) || (failMCP && wired != 0) {
				t.Fatalf("completed wiring callbacks: %d", wired)
			}
			if audit.closes.Load() != 1 {
				t.Fatal("watchdog audit not closed exactly once")
			}
			if engines != 1 || providers != 2 || reaps != 1 || audits != 1 || accountingOpens != 1 {
				t.Fatalf("wiring engines=%d providers=%d reap=%d audit=%d accounting=%d", engines, providers, reaps, audits, accountingOpens)
			}
			if store.closes.Load() != 1 || listener.closes.Load() != 1 || runtime.closed != 1 || catStops != 1 || catCtx.Err() == nil {
				t.Fatalf("cleanup store=%d listener=%d runtime=%d catalog=%d", store.closes.Load(), listener.closes.Load(), runtime.closed, catStops)
			}
			if !failMCP && (mcpStarts != 1 || mcpStops != 1 || mcpCtx.Err() == nil) {
				t.Fatal("MCP not drained")
			}
			if failMCP && (mcpStarts != 0 || mcpStops != 0) {
				t.Fatal("failed MCP started")
			}
			// Endpoints are installed at connect, SetConfig and SetRuntimeManager.
			if !reflect.DeepEqual(*runtimeCalls, []string{"probe", "remote", "endpoints", "warm", "endpoints", "endpoints"}) {
				t.Fatalf("unexpected runtime operations: %v", *runtimeCalls)
			}
			if !modelpolicy.Managed(context.Background()) {
				t.Fatal("shutdown failed to restore prior policy authority")
			}
			// Do not close the database here: a second Close would mask a leak.
			if err := telemetryStore.RecordSession(context.Background(), "after-shutdown"); err == nil {
				t.Fatal("telemetry database still open after shutdown")
			}

		})
	}
}
