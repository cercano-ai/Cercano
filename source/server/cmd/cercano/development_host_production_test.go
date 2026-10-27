package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/secrets"
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
	for p, body := range map[string]string{filepath.Join(state, "config.yaml"): "open_runtime: llama_server\nlocus_mode: open_only\n", filepath.Join(generation, "manifest.json"): "{\"test\":true}", filepath.Join(generation, "cercano"): "not executed"} {
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
	return developmentHostOptions{State: state, Generation: generation, Host: "production-test", RuntimeRoot: resources, Identity: &proto.LifecycleIdentity{Build: "prepared-test"}}, auth
}

func TestDevelopmentProductionInjectedGraph(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		t.Run(map[bool]string{false: "open", true: "cloud"}[cloud], func(t *testing.T) {
			o, auth := developmentProductionFixture(t)
			if cloud {
				if err := os.WriteFile(filepath.Join(o.State, "config.yaml"), []byte("open_runtime: llama_server\nlocus_mode: cloud_only\nactive_cloud_profile: test-cloud\ncloud_profiles:\n  - name: test-cloud\n    provider: openai\n    flavor: chat_completions\n    model: test-model\n    base_url: http://127.0.0.1:1/v1\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(filepath.Join(o.State, "config.yaml"))
			// A conflicting environment value must never replace the state snapshot.
			t.Setenv("CERCANO_OPEN_RUNTIME", "ollama")
			opened, connected, closed := 0, 0, 0
			memory := secrets.NewMemory()
			if err := memory.Set("test-cloud", "synthetic-not-a-credential"); err != nil {
				t.Fatal(err)
			}
			srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{
				openSecrets: func() (secrets.Store, error) { opened++; return memory, nil },
				connectRuntime: func(context.Context, config.Config, string) (localruntime.Manager, func(), error) {
					connected++
					return localruntime.NewManager(), func() { closed++ }, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			defer srv.Shutdown()
			if opened != 1 || connected != 1 {
				t.Fatalf("dependency counts %d/%d", opened, connected)
			}
			if srv.ConfigSnapshot().OpenRuntime != "llama_server" {
				t.Fatal("machine environment replaced snapshot")
			}
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
			cleanup()
			cleanup()
			if closed != 1 {
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
	_, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{
		openSecrets: func() (secrets.Store, error) { calls++; return nil, sentinel },
		connectRuntime: func(context.Context, config.Config, string) (localruntime.Manager, func(), error) {
			t.Fatal("runtime entered after failed credentials")
			return nil, nil, nil
		},
	})
	if !errors.Is(err, sentinel) || calls != 1 || cleanup != nil {
		t.Fatalf("error=%v calls=%d cleanup=%v", err, calls, cleanup != nil)
	}
}

func TestDevelopmentProductionPreflightBeforeOpener(t *testing.T) {
	o, auth := developmentProductionFixture(t)
	os.Remove(filepath.Join(o.State, "conversations.db"))
	_, _, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{openSecrets: func() (secrets.Store, error) { t.Fatal("opener entered before preflight"); return nil, nil }})
	if err == nil || strings.Contains(err.Error(), "dependencies required") {
		t.Fatalf("preflight result: %v", err)
	}
}

func TestDevelopmentProductionRejectsUnsupportedSnapshotBeforeDependencies(t *testing.T) {
	for _, snapshot := range []string{
		"open_runtime: ollama\nlocus_mode: open_only\n",
		"open_runtime: inprocess\nlocus_mode: open_only\n",
		"open_runtime: llama_server\nlocus_mode: invalid\n",
		"open_runtime: llama_server\nlocus_mode: cloud_only\n",
	} {
		t.Run(strings.ReplaceAll(snapshot, "\n", "/"), func(t *testing.T) {
			o, auth := developmentProductionFixture(t)
			if err := os.WriteFile(filepath.Join(o.State, "config.yaml"), []byte(snapshot), 0600); err != nil {
				t.Fatal(err)
			}
			_, _, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{
				openSecrets: func() (secrets.Store, error) { t.Fatal("unsupported config entered opener"); return nil, nil },
				connectRuntime: func(context.Context, config.Config, string) (localruntime.Manager, func(), error) {
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
	_, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, developmentProductionDependencies{
		openSecrets: func() (secrets.Store, error) { return secrets.NewMemory(), nil },
		connectRuntime: func(context.Context, config.Config, string) (localruntime.Manager, func(), error) {
			calls++
			return nil, nil, sentinel
		},
	})
	if !errors.Is(err, sentinel) || calls != 1 || cleanup != nil {
		t.Fatalf("error=%v calls=%d cleanup=%v", err, calls, cleanup != nil)
	}
}
