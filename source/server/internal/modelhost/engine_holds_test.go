package modelhost

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
	mistralengine "cercano/source/server/internal/engine/mistralrs"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
)

// startRuntimeHost runs a host whose provider is named runtime and reports a
// real OpenAI-compatible endpoint with confirmed capacity, so the inference
// engines can use its instances.
func startRuntimeHost(t *testing.T, runtime, token string) *testHost {
	t.Helper()
	chat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices": [{"message": {"role": "assistant", "content": "ok"}}]}`))
	}))
	t.Cleanup(chat.Close)
	provider := localruntimetest.NewFakeProvider(runtime, localruntime.ModelRecord{
		ID: "m1", Runtime: runtime, DisplayName: "m1", Path: "/models/m1.gguf", DownloadState: localruntime.Downloaded,
	})
	provider.Endpoint = chat.URL
	provider.Context = localruntime.ContextCapacity{ConfirmedTokens: 8192, ConfirmedAt: time.Now()}
	mgr := localruntime.NewManager()
	mgr.RegisterProvider(provider)
	srv := NewServer(mgr, ServerOptions{Token: token, Version: "v-test"})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	srv.Register(g)
	go g.Serve(lis)
	t.Cleanup(g.Stop)
	return &testHost{
		provider: provider, mgr: mgr, srv: srv, g: g,
		rec: Record{PID: os.Getpid(), Addr: lis.Addr().String(), Token: token, APIVersion: APIVersion, Version: "v-test", StartedAt: time.Now().UTC()},
	}
}

// engineComplete sends one completion for model m1 through runtime's engine.
func engineComplete(runtime string, m localruntime.Manager) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	switch runtime {
	case "llama_server":
		_, err = llamaengine.NewEngine(m).Complete(ctx, "m1", "hi", "", engine.GenOptions{})
	case "mistralrs":
		_, err = mistralengine.NewEngine(m).Complete(ctx, "m1", "hi", "", engine.GenOptions{})
	}
	return err
}

// TestEngineReuseHoldsSharedInstance: agent B finds agent A's running model
// through its engine and never starts it itself. A stopping the model from
// its runtime dashboard, or A shutting down, must not stop the model B is
// using: B's engine took a hold when it reused the instance.
func TestEngineReuseHoldsSharedInstance(t *testing.T) {
	for _, runtime := range []string{"llama_server", "mistralrs"} {
		t.Run(runtime, func(t *testing.T) {
			dir := t.TempDir()
			host := startRuntimeHost(t, runtime, "tok-"+runtime)
			host.publish(t, dir)
			a := newTestRemote(t, remoteOpts(dir, nil))
			b := newTestRemote(t, remoteOpts(dir, nil))
			ctx := context.Background()

			inst, err := a.Start(ctx, localruntime.StartRequest{Runtime: runtime, ModelID: "m1"})
			if err != nil {
				t.Fatalf("A Start: %v", err)
			}
			// B uses A's instance only through its engine.
			if err := engineComplete(runtime, b); err != nil {
				t.Fatalf("B engine request: %v", err)
			}
			if n := host.provider.StartCalls(); n != 1 {
				t.Fatalf("provider start calls = %d, want 1 (B must reuse A's instance)", n)
			}

			// A stops the model from its runtime dashboard: only A's hold goes.
			if err := a.Stop(ctx, localruntime.StopRequest{InstanceID: inst.ID}); err != nil {
				t.Fatalf("A Stop: %v", err)
			}
			if n := host.provider.StopCalls(); n != 0 {
				t.Fatalf("A's Stop stopped the model B is using (stop calls = %d)", n)
			}

			// A takes the model again, then shuts down: closing releases A's
			// holds, and B's hold keeps the model running.
			if _, err := a.Start(ctx, localruntime.StartRequest{Runtime: runtime, ModelID: "m1"}); err != nil {
				t.Fatalf("A Start again: %v", err)
			}
			if err := a.Close(); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, "A's holds released", func() bool { return len(host.srv.holds.heldBy(a.ClientID())) == 0 })
			if n := host.provider.StopCalls(); n != 0 {
				t.Fatalf("closing A stopped the model B is using (stop calls = %d)", n)
			}
			if err := engineComplete(runtime, b); err != nil {
				t.Fatalf("B engine request after A closed: %v", err)
			}
			if n := host.provider.StartCalls(); n != 1 {
				t.Fatalf("provider start calls = %d after A closed, want 1", n)
			}

			// B closing releases the last hold.
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, "model stopped after the last holder closed", func() bool { return host.provider.StopCalls() == 1 })
		})
	}
}

// newEngineCompleter returns a function that sends one completion for model
// m1 through a single runtime engine on m, so its hold cache persists across
// requests the way an agent's engine does.
func newEngineCompleter(runtime string, m localruntime.Manager) func() error {
	var complete func(context.Context) error
	switch runtime {
	case "llama_server":
		e := llamaengine.NewEngine(m)
		complete = func(ctx context.Context) error {
			_, err := e.Complete(ctx, "m1", "hi", "", engine.GenOptions{})
			return err
		}
	case "mistralrs":
		e := mistralengine.NewEngine(m)
		complete = func(ctx context.Context) error {
			_, err := e.Complete(ctx, "m1", "hi", "", engine.GenOptions{})
			return err
		}
	}
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return complete(ctx)
	}
}

// TestEngineReuseRetakesHoldAfterOwnStop: A and B both hold the model, B
// through its engine. B stops the model from its runtime dashboard, which
// releases only B's hold, so the model keeps running and no stop event
// arrives. B's engine must still take a new hold on its next request;
// otherwise A closing would stop the model B is using.
func TestEngineReuseRetakesHoldAfterOwnStop(t *testing.T) {
	for _, runtime := range []string{"llama_server", "mistralrs"} {
		t.Run(runtime, func(t *testing.T) {
			dir := t.TempDir()
			host := startRuntimeHost(t, runtime, "tok-"+runtime)
			host.publish(t, dir)
			a := newTestRemote(t, remoteOpts(dir, nil))
			b := newTestRemote(t, remoteOpts(dir, nil))
			ctx := context.Background()
			completeB := newEngineCompleter(runtime, b)

			inst, err := a.Start(ctx, localruntime.StartRequest{Runtime: runtime, ModelID: "m1"})
			if err != nil {
				t.Fatalf("A Start: %v", err)
			}
			if err := completeB(); err != nil {
				t.Fatalf("B engine request: %v", err)
			}
			if got := host.srv.holds.heldBy(b.ClientID()); len(got) != 1 {
				t.Fatalf("B holds %v after its engine reused the model, want 1 hold", got)
			}

			// B stops the model through its manager: only B's hold goes.
			if err := b.Stop(ctx, localruntime.StopRequest{InstanceID: inst.ID}); err != nil {
				t.Fatalf("B Stop: %v", err)
			}
			if n := host.provider.StopCalls(); n != 0 {
				t.Fatalf("B's Stop stopped the model A holds (stop calls = %d)", n)
			}
			if got := host.srv.holds.heldBy(b.ClientID()); len(got) != 0 {
				t.Fatalf("B holds %v after its Stop, want none", got)
			}

			// B's engine uses the model again and must hold it again.
			if err := completeB(); err != nil {
				t.Fatalf("B engine request after its Stop: %v", err)
			}
			if got := host.srv.holds.heldBy(b.ClientID()); len(got) != 1 || got[0] != inst.ID {
				t.Fatalf("B holds %v after reusing the model again, want [%s]", got, inst.ID)
			}

			// A closes: B's hold keeps the model running.
			if err := a.Close(); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, "A's holds released", func() bool { return len(host.srv.holds.heldBy(a.ClientID())) == 0 })
			if n := host.provider.StopCalls(); n != 0 {
				t.Fatalf("closing A stopped the model B is using (stop calls = %d)", n)
			}
			if err := completeB(); err != nil {
				t.Fatalf("B engine request after A closed: %v", err)
			}
			if n := host.provider.StartCalls(); n != 1 {
				t.Fatalf("provider start calls = %d, want 1", n)
			}

			// B closing releases the last hold.
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, "model stopped after the last holder closed", func() bool { return host.provider.StopCalls() == 1 })
		})
	}
}
