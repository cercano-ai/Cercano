package modelhost

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
)

// Count the manager boundary, not only successful provider Stop calls.
type loadedHoldManager struct {
	localruntime.Manager
	stops atomic.Int64
}

func (m *loadedHoldManager) Stop(ctx context.Context, req localruntime.StopRequest) error {
	m.stops.Add(1)
	return m.Manager.Stop(ctx, req)
}

// This is loaded-instance HOLD acceptance, NOT real model/runtime inference or
// compiled dev-host retirement acceptance. All resources belong to this test.
func TestProtectedLoadedInstanceABHoldAcceptance(t *testing.T) {
	for _, releaseA := range []string{"close", "stop-then-close"} {
		t.Run(releaseA, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var completions atomic.Int64
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				completions.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"synthetic-ok"}}]}`))
			}))
			t.Cleanup(endpoint.Close)
			provider := localruntimetest.NewFakeProvider("llama_server", localruntime.ModelRecord{
				ID: "m1", Runtime: "llama_server", Path: "/synthetic/not-a-real-model.gguf", DownloadState: localruntime.Downloaded,
			})
			provider.Endpoint = endpoint.URL
			provider.Context = localruntime.ContextCapacity{ConfirmedTokens: 8192, ConfirmedAt: time.Now()}
			manager := localruntime.NewManager()
			manager.RegisterProvider(provider)
			counted := &loadedHoldManager{Manager: manager}
			req := localruntime.StartRequest{Runtime: "llama_server", ModelID: "m1"}
			// Seed before the server/clients exist: protected clients may only reuse.
			loaded, err := counted.Start(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			srv := NewServer(counted, ServerOptions{Token: "isolated-hold-token", CoexistenceProtection: true})
			lis, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			g := grpc.NewServer()
			srv.Register(g)
			go g.Serve(lis)
			t.Cleanup(g.Stop)
			dir := t.TempDir()
			if err := WriteRecord(dir, Record{PID: os.Getpid(), Addr: lis.Addr().String(), Token: "isolated-hold-token", APIVersion: APIVersion, StartedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			opts := remoteOpts(dir, func(context.Context) error { t.Error("unexpected launch"); return ErrCoexistenceProtected })
			opts.CoexistenceProtection = true
			a := newTestRemote(t, opts)
			b := newTestRemote(t, opts)
			assertCounts := func(stage string, holds int, stops int64) {
				t.Helper()
				if got := srv.holds.count(loaded.ID); got != holds {
					t.Fatalf("%s: holds=%d want %d", stage, got, holds)
				}
				if got := counted.stops.Load(); got != stops {
					t.Fatalf("%s: manager Stops=%d want %d", stage, got, stops)
				}
				if got := provider.StartCalls(); got != 1 {
					t.Fatalf("%s: provider Starts=%d want seed only", stage, got)
				}
			}
			assertCounts("seed", 0, 0)
			for _, client := range []*Remote{a, b} {
				reused, err := client.Start(ctx, req)
				if err != nil || reused.ID != loaded.ID {
					t.Fatalf("reuse: instance=%+v err=%v", reused, err)
				}
				if !srv.holds.has(client.ClientID(), loaded.ID) {
					t.Fatal("client missing own hold")
				}
			}
			assertCounts("A+B own", 2, 0)
			completeB := newEngineCompleter("llama_server", b)
			if err := completeB(); err != nil {
				t.Fatal(err)
			}
			assertCounts("B usable before A release", 2, 0)
			if releaseA == "stop-then-close" {
				if err := a.Stop(ctx, localruntime.StopRequest{InstanceID: loaded.ID}); err != nil {
					t.Fatal(err)
				}
				assertCounts("A Stop", 1, 0)
			}
			if err := a.Close(); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, "A detached and hold released", func() bool {
				return srv.attachedClients(a.ClientID()) == 0 && !srv.holds.has(a.ClientID(), loaded.ID)
			})
			assertCounts("A closed", 1, 0)
			if srv.attachedClients(b.ClientID()) != 1 || !srv.holds.has(b.ClientID(), loaded.ID) || srv.draining.Load() {
				t.Fatal("B attachment/hold or host availability disturbed")
			}
			if err := completeB(); err != nil {
				t.Fatalf("B completion after A close: %v", err)
			}
			if got := completions.Load(); got != 2 {
				t.Fatalf("synthetic completions=%d want 2", got)
			}
			assertCounts("B usable after A release", 1, 0)
			instances, err := b.Instances(ctx)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, inst := range instances {
				if inst.ID == loaded.ID && inst.Endpoint == endpoint.URL && inst.State != localruntime.InstanceStopped {
					found = true
				}
			}
			if !found {
				t.Fatalf("original loaded instance not visible to B: %+v", instances)
			}
			// Final own release is allowed even on the protected server.
			if err := b.Stop(ctx, localruntime.StopRequest{InstanceID: loaded.ID}); err != nil {
				t.Fatal(err)
			}
			assertCounts("B final release", 0, 1)
			instances, err = b.Instances(ctx)
			if err != nil {
				t.Fatal(err)
			}
			stopped := false
			for _, inst := range instances {
				if inst.ID == loaded.ID && inst.State == localruntime.InstanceStopped {
					stopped = true
				}
			}
			if !stopped {
				t.Fatalf("final release did not stop original instance: %+v", instances)
			}
			if got := provider.StopCalls(); got != 1 {
				t.Fatalf("provider Stops=%d want 1", got)
			}
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, "B detached", func() bool { return srv.attachedClients(b.ClientID()) == 0 })
			assertCounts("all detached", 0, 1)
		})
	}
}
