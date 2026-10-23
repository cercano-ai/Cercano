package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"

	"cercano/source/server/internal/engine"
	llamaengine "cercano/source/server/internal/engine/llamaserver"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/internal/modelhost"
)

// startSharedModelHost runs a model host with a llama_server provider whose
// instances serve a stub OpenAI endpoint, publishes its record in a fresh run
// directory and returns that directory with the provider.
func startSharedModelHost(t *testing.T) (string, *localruntimetest.FakeProvider) {
	t.Helper()
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices": [{"message": {"role": "assistant", "content": "ok"}}]}`))
	}))
	t.Cleanup(chat.Close)
	provider := localruntimetest.NewFakeProvider("llama_server", localruntime.ModelRecord{
		ID: "m1", Runtime: "llama_server", DisplayName: "m1", Path: "/models/m1.gguf", DownloadState: localruntime.Downloaded,
	})
	provider.Endpoint = chat.URL
	provider.Context = localruntime.ContextCapacity{ConfirmedTokens: 8192, ConfirmedAt: time.Now()}
	mgr := localruntime.NewManager()
	mgr.RegisterProvider(provider)
	host := modelhost.NewServer(mgr, modelhost.ServerOptions{Token: "tok-shared", Version: "test"})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	host.Register(g)
	go g.Serve(lis)
	t.Cleanup(g.Stop)
	dir := t.TempDir()
	if err := modelhost.WriteRecord(dir, modelhost.Record{PID: os.Getpid(), Addr: lis.Addr().String(), Token: "tok-shared", APIVersion: modelhost.APIVersion}); err != nil {
		t.Fatal(err)
	}
	return dir, provider
}

func connectSharedRemote(t *testing.T, dir string) *modelhost.Remote {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := modelhost.NewRemote(ctx, modelhost.RemoteOptions{Ensure: modelhost.EnsureOptions{RunDir: dir, Timeout: 5 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestAgentShutdownLeavesSharedModelToOtherAgent: agent A starts a model in
// the shared host, agent B uses it only through its engine, then A shuts down
// the way the agent process does (Server.Shutdown, then closing its model
// host connection). The model B is using must keep running until B closes.
func TestAgentShutdownLeavesSharedModelToOtherAgent(t *testing.T) {
	dir, provider := startSharedModelHost(t)
	a := connectSharedRemote(t, dir)
	b := connectSharedRemote(t, dir)
	ctx := context.Background()

	if _, err := a.Start(ctx, localruntime.StartRequest{Runtime: "llama_server", ModelID: "m1"}); err != nil {
		t.Fatalf("A Start: %v", err)
	}
	engB := llamaengine.NewEngine(b)
	if _, err := engB.Complete(ctx, "m1", "hi", "", engine.GenOptions{}); err != nil {
		t.Fatalf("B engine request: %v", err)
	}

	srvA := &Server{}
	srvA.SetRuntimeManager(a)
	srvA.Shutdown()
	if n := provider.StopCalls(); n != 0 {
		t.Fatalf("A's shutdown stopped the model B is using (stop calls = %d)", n)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	// Let the host process A's detach before checking.
	time.Sleep(100 * time.Millisecond)
	if n := provider.StopCalls(); n != 0 {
		t.Fatalf("closing A stopped the model B is using (stop calls = %d)", n)
	}
	if _, err := engB.Complete(ctx, "m1", "hi", "", engine.GenOptions{}); err != nil {
		t.Fatalf("B engine request after A shut down: %v", err)
	}
	if n := provider.StartCalls(); n != 1 {
		t.Fatalf("provider start calls = %d, want 1", n)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "model stopped after the last holder closed", func() bool { return provider.StopCalls() == 1 })
}

// countingSharedManager is a shared manager that counts Stop calls.
type countingSharedManager struct {
	*localruntime.InMemoryManager
	stops int
}

func (m *countingSharedManager) HoldEpoch() uint64 { return 1 }

func (m *countingSharedManager) Stop(ctx context.Context, req localruntime.StopRequest) error {
	m.stops++
	return m.InMemoryManager.Stop(ctx, req)
}

// TestServerShutdownLeavesSharedInstancesToHolders: with a shared manager the
// agent stops nothing at shutdown. Closing the manager afterwards releases
// exactly this agent's holds; a Stop per listed instance would also reach
// instances only other agents hold.
func TestServerShutdownLeavesSharedInstancesToHolders(t *testing.T) {
	mgr := &countingSharedManager{InMemoryManager: localruntime.NewManager()}
	mgr.RegisterProvider(&shutdownRuntimeProvider{})
	mgr.UpdateInstance(localruntime.InstanceRecord{ID: "mistralrs:abc:1", Runtime: "mistralrs", ModelID: "m", State: localruntime.InstanceRunning})
	srv := &Server{}
	srv.SetRuntimeManager(mgr)
	srv.Shutdown()
	if mgr.stops != 0 {
		t.Fatalf("shutdown called Stop %d time(s) on a shared manager", mgr.stops)
	}
}
