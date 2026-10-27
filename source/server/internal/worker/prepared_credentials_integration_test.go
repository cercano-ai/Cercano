package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/capabilities"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	providerssvc "cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/llm/openai"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// The host credential backend is explicitly fake even when the test process
// disables all native credential owners.
type preparedFakeConfig struct {
	cfgsvc.Service
	store secrets.Store
}

func (s preparedFakeConfig) Secrets() secrets.Store { return s.store }

// Build only an HTTP fake provider, bypassing production/native factories.
func preparedFakeProviders(start *proto.StartTurn) (providerssvc.Resolver, error) {
	cfg := ConfigFromSnapshot(start.GetConfig())
	prof := cfg.CloudProfiles[0]
	key := "fake-key"
	return &workerResolver{cfgSvc: cfgsvc.New("", cfg, secrets.NewMemory()), cloudProv: openai.NewClient(openai.Config{BaseURL: prof.BaseURL, APIKey: key, Model: prof.Model, Backend: prof.Backend})}, nil
}

type preparedCredentialAuthority struct {
	contexts atomic.Int32
	closes   atomic.Int32
}

func (a *preparedCredentialAuthority) Context(ctx context.Context, _ string) (context.Context, error) {
	a.contexts.Add(1)
	return ctx, nil
}
func (a *preparedCredentialAuthority) Close(error) { a.closes.Add(1) }

type preparedCredentialStream struct {
	grpc.ServerStream
	ctx         context.Context
	credentials *atomic.Int32
}

func (s *preparedCredentialStream) SendMsg(m any) error {
	if msg, ok := m.(*proto.WorkerToHost); ok && msg.GetCredRequest() != nil {
		s.credentials.Add(1)
	}
	return s.ServerStream.SendMsg(m)
}

func (s *preparedCredentialStream) Context() context.Context { return s.ctx }

// Exercise the real RunTurn -> buildDeps -> full capability registry path.
// Explicit injection is authoritative even for prepared turns. No test enables
// credentials around openWorkerSecrets or calls that production opener.
func TestPreparedCredentialsRealWorkerIsolation(t *testing.T) {
	for _, policy := range []string{"valid", "missing", "malformed", "duplicate", "empty"} {
		t.Run(policy, func(t *testing.T) {
			var httpCalls, openerCalls, credentialRequests atomic.Int32
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				httpCalls.Add(1)
				if r.Header.Get("Authorization") != "Bearer host-only-key" {
					t.Errorf("provider did not receive host credential")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"id\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"isolated\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
			}))
			defer endpoint.Close()
			store := secrets.NewMemory()
			ws := NewWithSecretStore(store, WithPrepared())
			// Counting injection replaces the constructor's closure, not the native
			// opener. Leave BOTH factories nil so assembly cannot be bypassed.
			ws.secretOpener = func() capabilities.SecretStore { openerCalls.Add(1); return store }
			var preparedSeen, ordinarySeen atomic.Int32
			listener := bufconn.Listen(1 << 20)
			server := grpc.NewServer(grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				md, _ := metadata.FromIncomingContext(stream.Context())
				md = md.Copy()
				if values := md.Get(preparedCredentialPolicyHeader); len(values) != 0 {
					if len(values) != 1 || values[0] != "true" {
						t.Errorf("host policy = %v", values)
					}
					preparedSeen.Add(1)
					switch policy {
					case "missing":
						md.Delete(preparedCredentialPolicyHeader)
					case "malformed":
						md.Set(preparedCredentialPolicyHeader, "not-a-policy")
					case "duplicate":
						md.Set(preparedCredentialPolicyHeader, "true", "true")
					case "empty":
						md.Set(preparedCredentialPolicyHeader, "")
					}
				} else {
					ordinarySeen.Add(1)
				}
				return handler(srv, &preparedCredentialStream{ServerStream: stream, credentials: &credentialRequests, ctx: metadata.NewIncomingContext(stream.Context(), md)})
			}))
			proto.RegisterWorkerServer(server, ws)
			go func() { _ = server.Serve(listener) }()
			defer server.Stop()
			defer listener.Close()
			cfg := config.Defaults()
			cfg.LocusMode = "cloud_only"
			cfg.ActiveCloudProfile = "fixture"
			cfg.CloudModel = "fixture-model"
			cfg.CloudProfiles = []config.CloudProfile{{Name: "fixture", Provider: "openai", Backend: "openai", Flavor: "chat_completions", BaseURL: endpoint.URL, Model: "fixture-model"}}
			hostStore := secrets.NewMemory()
			if err := hostStore.Set("fixture", "host-only-key"); err != nil {
				t.Fatal(err)
			}
			service := preparedFakeConfig{Service: cfgsvc.New("", cfg, hostStore), store: hostStore}
			host := newWorkerRunnerWithDial(nil, service, nil, testDialUnix(listener))
			authority := &preparedCredentialAuthority{}
			host.prepared = true
			host.SetTurnAuthority(func(context.Context, string) (TurnAuthority, error) { return authority, nil })
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			result, err := host.RunTurn(ctx, runner.Request{ConversationID: "prepared", Input: "hello", WorkDir: t.TempDir()}, nil, nil, nil)
			if policy != "valid" {
				if err == nil || openerCalls.Load() != 0 || httpCalls.Load() != 0 {
					t.Fatalf("explicit prepared session did not fail closed after %s metadata: err=%v capability opens=%d provider requests=%d result=%q", policy, err, openerCalls.Load(), httpCalls.Load(), result.FinalText)
				}
				return
			}
			if err != nil || result.FinalText != "isolated" {
				t.Fatalf("prepared result=%+v err=%v", result, err)
			}
			if preparedSeen.Load() != 1 || openerCalls.Load() != 1 || httpCalls.Load() != 1 || credentialRequests.Load() != 1 || authority.contexts.Load() != 1 || authority.closes.Load() != 1 {
				t.Fatalf("prepared path: metadata=%d opens=%d http=%d authority=%d/%d", preparedSeen.Load(), openerCalls.Load(), httpCalls.Load(), authority.contexts.Load(), authority.closes.Load())
			}
			// An ordinary sibling is a separate server instance, not a downgraded
			// session on a prepared process.
			ordinaryListener := bufconn.Listen(1 << 20)
			ordinaryServer := grpc.NewServer(grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				md, _ := metadata.FromIncomingContext(stream.Context())
				if len(md.Get(preparedCredentialPolicyHeader)) != 0 {
					t.Error("ordinary sibling carried prepared metadata")
				}
				ordinarySeen.Add(1)
				return handler(srv, stream)
			}))
			ordinaryWorker := NewWithSecretStore(store)
			ordinaryWorker.secretOpener = ws.secretOpener
			proto.RegisterWorkerServer(ordinaryServer, ordinaryWorker)
			go func() { _ = ordinaryServer.Serve(ordinaryListener) }()
			defer ordinaryServer.Stop()
			defer ordinaryListener.Close()
			sibling := newWorkerRunnerWithDial(nil, service, nil, testDialUnix(ordinaryListener))
			result, err = sibling.RunTurn(ctx, runner.Request{ConversationID: "ordinary", Input: "hello", WorkDir: t.TempDir()}, nil, nil, nil)
			if err != nil || result.FinalText != "isolated" || ordinarySeen.Load() != 1 || preparedSeen.Load() != 1 || openerCalls.Load() != 2 {
				t.Fatalf("ordinary sibling leaked policy: result=%+v err=%v ordinary=%d prepared=%d opens=%d", result, err, ordinarySeen.Load(), preparedSeen.Load(), openerCalls.Load())
			}
			// Identity only: never enter the native default to check ordinary behavior.
			if reflect.ValueOf(workerSecretOpener(context.Background(), nil)).Pointer() != reflect.ValueOf(openWorkerSecrets).Pointer() {
				t.Fatal("ordinary default changed")
			}
		})
	}
}
