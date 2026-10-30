package main

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/grpc"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/watchdog"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/metadata"
)

func developmentProductionFixture(t *testing.T) (developmentHostOptions, *devhostauth.Auth) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "state")
	generation := filepath.Join(root, "generation")
	for _, p := range []string{state, generation} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	resources, err := os.MkdirTemp("/tmp", "cdp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(resources) })
	resources, err = filepath.EvalSymlinks(resources)
	if err != nil {
		t.Fatal(err)
	}
	if err := conversation.PrepareDevelopmentStore(context.Background(), filepath.Join(state, "conversations.db")); err != nil {
		t.Fatal(err)
	}
	for p, body := range map[string]string{filepath.Join(state, "config.yaml"): "worker_idle_timeout_seconds: -1\nopen_runtime: llama_server\nlocus_mode: open_only\n", filepath.Join(generation, "manifest.json"): "{\"test\":true}", filepath.Join(generation, "cercano"): "not executed"} {
		if err := os.WriteFile(p, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	token := filepath.Join(root, "token")
	if err := devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load("production-test", token)
	if err != nil {
		t.Fatal(err)
	}
	o := developmentHostOptions{State: state, Generation: generation, Host: "production-test", RuntimeRoot: resources, Identity: &proto.LifecycleIdentity{Build: "prepared-test", Host: "production-test", Store: filepath.Join(state, "conversations.db"), Contract: conversation.DevelopmentCoexistenceDeclaration().Contract, Schema: int32(conversation.DevelopmentCoexistenceDeclaration().SchemaVersion)}}
	o.ConfigAuthority = filepath.Join(state, "config.yaml")
	// Lower-level constructor fixture only: this does not exercise or attest
	// launcher/executable validation. Real acceptance uses the actual launcher.
	binding := developmentBinding(o)
	o.startupAdmission = productionStartupAdmission{
		paths:               productionAuthorityPaths{config: o.ConfigAuthority, conversations: o.Identity.Store},
		generationPreflight: true, bounded: &binding,
	}
	return o, auth
}

func TestDevelopmentProductionInjectedGraph(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		t.Run(map[bool]string{false: "open", true: "cloud"}[cloud], func(t *testing.T) {
			o, auth := developmentProductionFixture(t)
			if cloud {
				if err := os.WriteFile(filepath.Join(o.State, "config.yaml"), []byte("worker_idle_timeout_seconds: -1\nopen_runtime: llama_server\nlocus_mode: cloud_only\nactive_cloud_profile: test-cloud\ncloud_profiles:\n  - name: test-cloud\n    provider: openai\n    flavor: chat_completions\n    model: test-model\n    base_url: http://127.0.0.1:1/v1\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(filepath.Join(o.State, "config.yaml"))
			// Ordinary supported overrides are resolved without rewriting disk.
			t.Setenv("CERCANO_PORT", "17777")
			opened, connected, closed := 0, 0, 0
			memory := secrets.NewMemory()
			if err := memory.Set("test-cloud", "synthetic-not-a-credential"); err != nil {
				t.Fatal(err)
			}
			srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, enabledDevelopmentFakes(t, developmentProductionDependencies{
				openSecrets: func() (secrets.Store, error) { opened++; return memory, nil },
				connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func() error, error) {
					connected++
					return localruntime.NewManager(), func() error { closed++; return nil }, nil
				},
			}))
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup.Wait(context.Background())

			wantRuntime := 1
			if cloud {
				wantRuntime = 0
			}
			if opened != 1 || connected != wantRuntime {
				t.Fatalf("dependency counts %d/%d", opened, connected)
			}
			if srv.ConfigSnapshot().OpenRuntime != "llama_server" || srv.ConfigSnapshot().Port != "17777" {
				t.Fatal("ordinary config resolution lost")
			}
			// Exercise the exact authenticated readiness/list bridge, not a direct call.
			transport := grpc.NewServer(auth.ServerOptions()...)
			proto.RegisterAgentServer(transport, srv)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			defer transport.Stop()
			go transport.Serve(listener)
			probe, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, err := developmentDialIdentity(probe, listener.Addr().String(), auth, o.Identity)
			if err != nil {
				t.Fatalf("authenticated readiness/list: %v", err)
			}
			conn.Close()
			listed, err := srv.ListTools(context.Background(), &proto.ListToolsRequest{})
			if err != nil || len(listed.Tools) == 0 {
				t.Fatalf("missing real registry: %v %v", listed, err)
			}
			read, ok := srv.ToolRegistry().Get("Read")
			if !ok {
				t.Fatal("Read builtin missing")
			}
			// Exercise real durable reload and the exact foreground admission bridge,
			// without calling an inference provider or starting a child process.
			owner, err := srv.Retain(context.Background(), "foreground")
			if err != nil {
				t.Fatal(err)
			}
			bound := metadata.NewIncomingContext(context.Background(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "terminal"))
			admitted, attachment, ticket, err := srv.BeginTerminalStream(bound, "foreground", broker.ForegroundWork)
			if err != nil {
				t.Fatal(err)
			}
			args, err := json.Marshal(map[string]string{"path": filepath.Join(o.State, "config.yaml")})
			if err != nil {
				t.Fatal(err)
			}
			result, err := read.Execute(admitted, args)
			if err != nil || result == nil || result.Text != string(before) {
				t.Fatalf("admitted real Read result=%v error=%v", result, err)
			}
			if err := ticket.Complete(); err != nil {
				t.Fatal(err)
			}
			attachment.Detach()
			if err := srv.ReleaseRetained(context.Background(), owner, nil); err != nil {
				t.Fatal(err)
			}
			if err := srv.RetireCoexistence(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := cleanup.Wait(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := cleanup.Wait(context.Background()); err != nil {
				t.Fatal(err)
			}
			if closed != wantRuntime {
				t.Fatalf("runtime closed %d times", closed)
			}
			after, _ := os.ReadFile(filepath.Join(o.State, "config.yaml"))
			if string(before) != string(after) {
				t.Fatal("configuration was rewritten")
			}
		})
	}
}

func TestDevelopmentProductionOpenerFailsOnceWithoutFallback(t *testing.T) {
	o, auth := developmentProductionFixture(t)
	calls := 0
	sentinel := errors.New("injected opener failure")
	_, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, enabledDevelopmentFakes(t, developmentProductionDependencies{
		openSecrets: func() (secrets.Store, error) { calls++; return nil, sentinel },
		connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func() error, error) {
			t.Fatal("runtime entered after failed credentials")
			return nil, nil, nil
		},
	}, developmentBoundaryCounts{opener: 1}))
	if cleanup != nil {
		_ = cleanup.Wait(context.Background())
	}
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("error=%v calls=%d cleanup=%v", err, calls, cleanup != nil)
	}
}

func TestDevelopmentProductionMissingAdmissionBeforeOpener(t *testing.T) {
	o, auth := developmentProductionFixture(t)
	o.startupAdmission = productionStartupAdmission{}
	_, _, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{openSecrets: func() (secrets.Store, error) { t.Fatal("opener entered before preflight"); return nil, nil }})
	if err == nil || strings.Contains(err.Error(), "dependencies required") {
		t.Fatalf("preflight result: %v", err)
	}
}

func TestDevelopmentProductionRejectsUnsupportedSnapshotBeforeDependencies(t *testing.T) {
	for _, snapshot := range []string{
		"open_runtime: ollama\nlocus_mode: open_only\n",
		"open_runtime: inprocess\nlocus_mode: open_only\n",
		"worker_idle_timeout_seconds: -1\nopen_runtime: llama_server\nlocus_mode: invalid\n",
		"worker_idle_timeout_seconds: -1\nopen_runtime: llama_server\nlocus_mode: cloud_only\n",
	} {
		t.Run(strings.ReplaceAll(snapshot, "\n", "/"), func(t *testing.T) {
			o, auth := developmentProductionFixture(t)
			if err := os.WriteFile(filepath.Join(o.State, "config.yaml"), []byte(snapshot), 0600); err != nil {
				t.Fatal(err)
			}
			_, _, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{
				openSecrets: func() (secrets.Store, error) { t.Fatal("unsupported config entered opener"); return nil, nil },
				connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func() error, error) {
					t.Fatal("unsupported config entered runtime")
					return nil, nil, nil
				},
			})
			if err == nil {
				t.Fatal("unsupported snapshot accepted")
			}
		})
	}
}

func TestDevelopmentProductionRuntimeErrorWithoutFallback(t *testing.T) {
	o, auth := developmentProductionFixture(t)
	calls := 0
	sentinel := errors.New("protected modelhost unavailable")
	_, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, enabledDevelopmentFakes(t, developmentProductionDependencies{
		openSecrets: func() (secrets.Store, error) { return secrets.NewMemory(), nil },
		connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func() error, error) {
			calls++
			return nil, nil, sentinel
		},
	}, developmentBoundaryCounts{opener: 1, runtime: 1}))
	if cleanup != nil {
		_ = cleanup.Wait(context.Background())
	}
	if !errors.Is(err, sentinel) || calls != 1 {
		t.Fatalf("error=%v calls=%d cleanup=%v", err, calls, cleanup != nil)
	}
}

// A failing server-owned diagnostic must not be lost by the prepared adapter.
type failingDevelopmentAudit struct {
	startupAudit
	err error
}

func (a *failingDevelopmentAudit) Close() error { a.closes.Add(1); return a.err }

func TestDevelopmentProductionCleanupShutdownError(t *testing.T) {
	for _, resourceFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "shutdown-only", true: "joined-resource-error"}[resourceFailure], func(t *testing.T) {
			o, auth := developmentProductionFixture(t)
			if err := os.WriteFile(filepath.Join(o.State, "config.yaml"), []byte("worker_idle_timeout_seconds: -1\nopen_runtime: llama_server\nlocus_mode: open_only\nwatchdog:\n  enabled: true\n  audit:\n    enabled: true\n"), 0600); err != nil {
				t.Fatal(err)
			}
			sentinel := errors.New("audit close sentinel")
			var runtimeErr error
			if resourceFailure {
				runtimeErr = errors.New("runtime close sentinel")
			}
			audit := &failingDevelopmentAudit{err: sentinel}
			closed := 0
			srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, enabledDevelopmentFakes(t, developmentProductionDependencies{
				openSecrets: func() (secrets.Store, error) { return secrets.NewMemory(), nil },
				watchdog:    server.WatchdogDependencies{OpenAudit: func(string) (watchdog.AuditLogger, error) { return audit, nil }},
				connectRuntime: func(context.Context, config.Config, string, string) (localruntime.Manager, func() error, error) {
					return localruntime.NewManager(), func() error { closed++; return runtimeErr }, nil
				},
			}))
			if err != nil {
				t.Fatal(err)
			}
			// Prepared startup deliberately leaves supervision inactive. Initialize only
			// this injected audit owner; no turn or model call is made.
			srv.InitWatchdog()
			for i := 0; i < 2; i++ {
				err := cleanup.Wait(context.Background())
				st := cleanup.Status()
				control := cleanup.Control(true)
				if (runtimeErr != nil && !errors.Is(err, runtimeErr)) || !errors.Is(err, sentinel) || st.Complete || control.Complete || control.Phase != "failed" || !errors.Is(st.Err, sentinel) {
					t.Fatalf("cleanup error=%v status=%+v", err, st)
				}
			}
			if audit.closes.Load() != 1 || closed != 1 {
				t.Fatalf("close counts audit=%d runtime=%d", audit.closes.Load(), closed)
			}
		})
	}
}

// Constructor-level admission tests, not entrypoint acceptance.
func TestDevelopmentValidatedStartupBinding(t *testing.T) {
	o, auth := developmentProductionFixture(t)
	if err := o.startupAdmission.validateDevelopmentBinding(o); err != nil {
		t.Fatal(err)
	}
	canonical := canonicalGenerationStartup()
	canonical.authorities = productionAuthorityResolvers{
		configPath:       func() string { return o.ConfigAuthority },
		conversationPath: func() (string, error) { return o.Identity.Store, nil },
	}
	if _, err := canonical.admitActivation(context.Background()); err == nil {
		t.Fatal("canonical activation admitted")
	}
	for _, field := range []string{"absent", "state", "config", "store", "host", "build", "generation"} {
		t.Run(field, func(t *testing.T) {
			bad := o
			id := *o.Identity
			bad.Identity = &id
			switch field {
			case "absent":
				bad.startupAdmission = productionStartupAdmission{}
			case "state":
				bad.State += "-other"
			case "config":
				bad.ConfigAuthority += "-missing"
			case "store":
				bad.Identity.Store += "-other"
			case "host":
				bad.Host += "-other"
			case "build":
				bad.Identity.Build += "-other"
			case "generation":
				bad.Generation += "-other"
			}
			_, _, err := newProductionDevelopmentHostWithDependencies(context.Background(), bad, auth, developmentProductionDependencies{
				openSecrets: func() (secrets.Store, error) { t.Fatal("dependency entered before admission"); return nil, nil },
			})
			if err == nil || !strings.Contains(err.Error(), "binding required") {
				t.Fatalf("expected binding denial before config/dependencies: %v", err)
			}
		})
	}
}
