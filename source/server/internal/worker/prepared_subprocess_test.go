package worker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/capabilities"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Test executable entry only. Prepared identity comes from argv, never the
// test selector environment. Every dependency is fake and credentials disabled.
func init() {
	if os.Getenv("CERCANO_PREPARED_POLICY_TEST_CHILD") != "1" {
		return
	}
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" || len(os.Args) != 5 || os.Args[1] != "worker" || os.Args[2] != "--socket" || os.Args[4] != "--prepared" {
		os.Exit(94)
	}
	listener, err := net.Listen("unix", os.Args[3])
	if err != nil {
		os.Exit(95)
	}
	ws := NewWithSecretStore(secrets.NewMemory(), WithPrepared())
	ws.providerFactory = preparedFakeProviders
	ws.secretOpener = func() capabilities.SecretStore {
		f, err := os.OpenFile(os.Getenv("CERCANO_PREPARED_POLICY_TEST_COUNT"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}
		_, _ = f.WriteString("open\n")
		_ = f.Close()
		return secrets.NewMemory()
	}
	server := grpc.NewServer()
	proto.RegisterWorkerServer(server, ws)
	if server.Serve(listener) != nil {
		os.Exit(96)
	}
	os.Exit(0)
}

func TestPreparedCredentialPolicySubprocessPropagation(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("CERCANO_PREPARED_POLICY_TEST_CHILD", "1")
	var providerCalls atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer fake-key" {
			t.Error("missing host credential")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"fake\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer endpoint.Close()
	for _, probe := range []string{"missing", "malformed", "duplicate", "valid"} {
		t.Run(probe, func(t *testing.T) {
			options := policyFixture(t)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			binary, err := os.ReadFile(executable)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(options.GenerationDir, cercanoBinaryName), binary, 0700); err != nil {
				t.Fatal(err)
			}
			counter := filepath.Join(options.GenerationDir, "opens")
			t.Setenv("CERCANO_PREPARED_POLICY_TEST_COUNT", counter)
			policy, err := NewSpawnPolicy(options)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child, err := spawnWorkerWithPolicy(ctx, "policy", 1, policy)
			if err != nil {
				t.Fatal(err)
			}
			defer child.Kill()
			var headers atomic.Int32
			conn, err := grpc.NewClient("unix:"+child.socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
				md, _ := metadata.FromOutgoingContext(ctx)
				md = md.Copy()
				values := md.Get(preparedCredentialPolicyHeader)
				if len(values) != 1 || values[0] != "true" {
					t.Errorf("launch/session mismatch: %v", values)
				}
				headers.Add(1)
				switch probe {
				case "missing":
					md.Delete(preparedCredentialPolicyHeader)
				case "malformed":
					md.Set(preparedCredentialPolicyHeader, "false")
				case "duplicate":
					md.Set(preparedCredentialPolicyHeader, "true", "true")
				}
				return streamer(metadata.NewOutgoingContext(ctx, md), desc, cc, method, opts...)
			}))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			cfg := config.Defaults()
			cfg.LocusMode = "cloud_only"
			cfg.ActiveCloudProfile = "fixture"
			cfg.CloudProfiles = []config.CloudProfile{{Name: "fixture", Provider: "openai", Backend: "openai", Flavor: "chat_completions", BaseURL: endpoint.URL, Model: "fake-model"}}
			store := secrets.NewMemory()
			if err := store.Set("fixture", "fake-key"); err != nil {
				t.Fatal(err)
			}
			host := newWorkerRunnerWithDial(nil, preparedFakeConfig{Service: cfgsvc.New("", cfg, store), store: store}, nil, func(context.Context) (*grpc.ClientConn, error) { return conn, nil })
			host.prepared = true
			before := providerCalls.Load()
			result, err := host.RunTurn(ctx, runner.Request{ConversationID: "policy", Input: "hello", WorkDir: options.GenerationDir}, nil, nil, nil)
			opens, readErr := os.ReadFile(counter)
			if headers.Load() != 1 {
				t.Fatalf("session headers=%d", headers.Load())
			}
			if probe == "valid" {
				if err != nil || result.FinalText != "fake" || string(opens) != "open\n" || providerCalls.Load() != before+1 {
					t.Fatalf("result=%+v err=%v opens=%q read=%v calls=%d", result, err, opens, readErr, providerCalls.Load())
				}
			} else if err == nil || !os.IsNotExist(readErr) || providerCalls.Load() != before {
				t.Fatalf("policy bypass: err=%v opens=%q read=%v provider=%d", err, opens, readErr, providerCalls.Load())
			}
		})
	}
}
