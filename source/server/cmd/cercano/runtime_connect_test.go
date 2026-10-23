package main

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/crashlog"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/config"
)

const sharedTestModel = "shared-model"

// connectTestEnv isolates HOME (and with it the model host run directory)
// and clears any CERCANO_MODEL_HOST setting from the developer's shell.
func connectTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(modelHostEnv, "")
}

// fakeHostManager is the manager the in-process model host serves: a
// FakeProvider registered as llama_server, so agent warming reaches it.
func fakeHostManager() (*localruntime.InMemoryManager, *localruntimetest.FakeProvider) {
	mgr := localruntime.NewManager()
	fake := localruntimetest.NewFakeProvider("llama_server", localruntime.ModelRecord{ID: sharedTestModel, Runtime: "llama_server"})
	mgr.RegisterProvider(fake)
	return mgr, fake
}

// useInProcessModelHost makes a "launch" run runModelHost in this process
// over mgr, in the run directory the agent passed. It returns the number of
// launches. The host is cancelled and awaited at cleanup, after any closes
// registered later.
func useInProcessModelHost(t *testing.T, mgr localruntime.Manager) *atomic.Int32 {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var launches atomic.Int32
	var mu sync.Mutex
	var dones []chan error
	prev := launchModelHost
	launchModelHost = func(runDir string) func(context.Context) error {
		return func(context.Context) error {
			launches.Add(1)
			done := make(chan error, 1)
			mu.Lock()
			dones = append(dones, done)
			mu.Unlock()
			go func() {
				done <- runModelHost(ctx, modelHostOptions{
					RunDir:      runDir,
					Config:      config.Defaults(),
					IdleTimeout: time.Minute,
					Manager:     mgr,
				})
			}()
			return nil
		}
	}
	t.Cleanup(func() {
		launchModelHost = prev
		cancel()
		mu.Lock()
		defer mu.Unlock()
		for _, d := range dones {
			if err := waitDone(t, d, 10*time.Second); err != nil {
				t.Errorf("runModelHost: %v", err)
			}
		}
	})
	return &launches
}

// warmConfig makes llama_server the active runtime with sharedTestModel as
// its effective chat model, so connectRuntimeManager warms it.
func warmConfig() config.Config {
	cfg := config.Defaults()
	cfg.OpenRuntime = "llama_server"
	cfg.LlamaServer.Enabled = true
	cfg.MistralRS.Enabled = false
	cfg.Models.SetOverride("llama_server", config.TierEveryday, sharedTestModel)
	return cfg
}

// ollamaConfig activates no managed runtime, so an in-process manager warms
// nothing and launches no real runtime.
func ollamaConfig() config.Config {
	cfg := config.Defaults()
	cfg.OpenRuntime = "ollama"
	cfg.LlamaServer.Enabled = false
	cfg.LlamaServer.DefaultModel = ""
	cfg.MistralRS.Enabled = false
	cfg.MistralRS.DefaultModel = ""
	return cfg
}

func connect(t *testing.T, cfg config.Config, events *crashlog.Writer) (localruntime.Manager, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	m, closeFn, err := connectRuntimeManager(ctx, cfg, events)
	if err != nil {
		t.Fatalf("connectRuntimeManager: %v", err)
	}
	if closeFn == nil {
		t.Fatal("connectRuntimeManager returned a nil close function")
	}
	var once sync.Once
	closeOnce := func() { once.Do(closeFn) }
	t.Cleanup(closeOnce)
	return m, closeOnce
}

// waitWarmed waits until the host's log shows n completed warm-ups of the
// shared model, one per agent.
func waitWarmed(t *testing.T, mgr localruntime.Manager, n int) {
	t.Helper()
	want := warmedMessage("llama_server", sharedTestModel)
	deadline := time.Now().Add(10 * time.Second)
	for {
		logs, err := mgr.Logs(context.Background(), localruntime.LogRequest{})
		if err != nil {
			t.Fatalf("Logs: %v", err)
		}
		got := 0
		for _, e := range logs {
			if e.Message == want {
				got++
			}
		}
		if got >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("saw %d warm-ups of %s, want %d", got, sharedTestModel, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTwoAgentHostsShareOneModelHost(t *testing.T) {
	connectTestEnv(t)
	mgr, fake := fakeHostManager()
	launches := useInProcessModelHost(t, mgr)
	cfg := warmConfig()

	a, _ := connect(t, cfg, nil)
	b, _ := connect(t, cfg, nil)
	for name, m := range map[string]localruntime.Manager{"A": a, "B": b} {
		if _, ok := m.(*modelhost.Remote); !ok {
			t.Fatalf("agent %s manager is %T, want *modelhost.Remote", name, m)
		}
		eps, err := m.Endpoints(context.Background())
		if err != nil {
			t.Fatalf("agent %s Endpoints: %v", name, err)
		}
		want := localruntime.EndpointsFromConfig(cfg, openChatModel(cfg), openEmbeddingModel(cfg))
		if !reflect.DeepEqual(eps, want) {
			t.Fatalf("agent %s endpoints = %+v, want %+v", name, eps, want)
		}
	}

	waitWarmed(t, mgr, 2)
	if got := fake.StartCalls(); got != 1 {
		t.Fatalf("StartCalls = %d, want 1 (both agents share one model)", got)
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("model host launched %d times, want 1", got)
	}
}

func TestInprocessEscapeHatch(t *testing.T) {
	connectTestEnv(t)
	t.Setenv(modelHostEnv, "inprocess")
	prev := launchModelHost
	launchModelHost = func(string) func(context.Context) error {
		return func(context.Context) error {
			t.Error("model host launched despite CERCANO_MODEL_HOST=inprocess")
			return errors.New("unexpected launch")
		}
	}
	t.Cleanup(func() { launchModelHost = prev })

	m, _ := connect(t, ollamaConfig(), nil)
	if _, ok := m.(*localruntime.InMemoryManager); !ok {
		t.Fatalf("manager is %T, want *localruntime.InMemoryManager", m)
	}
}

func TestFallsBackWhenModelHostUnavailable(t *testing.T) {
	connectTestEnv(t)
	prev := launchModelHost
	launchModelHost = func(string) func(context.Context) error {
		return func(context.Context) error { return errors.New("launch refused") }
	}
	t.Cleanup(func() { launchModelHost = prev })

	logPath := filepath.Join(t.TempDir(), "crash.log")
	events, err := crashlog.NewWriter(logPath, version)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = events.Close() })

	m, _ := connect(t, ollamaConfig(), events)
	if _, ok := m.(*localruntime.InMemoryManager); !ok {
		t.Fatalf("manager is %T, want the in-process *localruntime.InMemoryManager", m)
	}

	entries, err := crashlog.TailEntries(logPath, 50)
	if err != nil {
		t.Fatalf("TailEntries: %v", err)
	}
	for _, e := range entries {
		if e.Kind == crashlog.KindRuntimeEvent && e.Event == crashlog.EventModelHostUnavailable {
			if !strings.Contains(e.Reason, "launch refused") {
				t.Fatalf("event reason %q does not carry the launch error", e.Reason)
			}
			return
		}
	}
	t.Fatalf("no %s event in crash log: %+v", crashlog.EventModelHostUnavailable, entries)
}

func TestAgentShutdownDoesNotStopSharedModel(t *testing.T) {
	connectTestEnv(t)
	mgr, fake := fakeHostManager()
	useInProcessModelHost(t, mgr)
	cfg := warmConfig()

	a, closeA := connect(t, cfg, nil)
	b, closeB := connect(t, cfg, nil)
	waitWarmed(t, mgr, 2)

	// Agent A shuts down the way startGRPCServer's cleanup does: the server
	// stops its runtime instances, then the runtime connection closes.
	srv := server.NewServer(nil, nil, nil, nil, nil)
	srv.SetRuntimeManager(a)
	srv.Shutdown()
	closeA()

	if got := fake.StopCalls(); got != 0 {
		t.Fatalf("StopCalls = %d after agent A shut down, want 0 while B holds the model", got)
	}
	insts, err := b.Instances(context.Background())
	if err != nil {
		t.Fatalf("B Instances: %v", err)
	}
	running := false
	for _, inst := range insts {
		if inst.ModelID == sharedTestModel && inst.State != localruntime.InstanceStopped {
			running = true
		}
	}
	if !running {
		t.Fatalf("shared model not running after A shut down: %+v", insts)
	}

	// Once B lets go too, the host stops the model.
	closeB()
	deadline := time.Now().Add(10 * time.Second)
	for fake.StopCalls() != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("StopCalls = %d after both agents closed, want 1", fake.StopCalls())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// shortenModelHostBudget shrinks the agent's model host connect budget so a
// hung host fails over quickly in tests.
func shortenModelHostBudget(t *testing.T, ensure, budget time.Duration) {
	t.Helper()
	prevEnsure, prevBudget := modelHostEnsureTimeout, modelHostConnectBudget
	modelHostEnsureTimeout, modelHostConnectBudget = ensure, budget
	t.Cleanup(func() { modelHostEnsureTimeout, modelHostConnectBudget = prevEnsure, prevBudget })
}

func TestConnectFallsBackWithinBudgetWhenHostNeverReady(t *testing.T) {
	for _, tc := range []struct {
		name           string
		ensure, budget time.Duration
	}{
		// Ensure gives up on the launched host first.
		{"ensure timeout", 300 * time.Millisecond, 500 * time.Millisecond},
		// The overall budget cuts off an Ensure that would wait longer.
		{"overall budget", 5 * time.Second, 300 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testConnectFallsBackWithinBudget(t, tc.ensure, tc.budget)
		})
	}
}

func testConnectFallsBackWithinBudget(t *testing.T, ensure, budget time.Duration) {
	connectTestEnv(t)
	shortenModelHostBudget(t, ensure, budget)
	// The launch "succeeds" but no host ever publishes a record.
	var launches atomic.Int32
	prev := launchModelHost
	launchModelHost = func(string) func(context.Context) error {
		return func(context.Context) error { launches.Add(1); return nil }
	}
	t.Cleanup(func() { launchModelHost = prev })

	logPath := filepath.Join(t.TempDir(), "crash.log")
	events, err := crashlog.NewWriter(logPath, version)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = events.Close() })

	start := time.Now()
	m, _ := connect(t, ollamaConfig(), events)
	elapsed := time.Since(start)
	if _, ok := m.(*localruntime.InMemoryManager); !ok {
		t.Fatalf("manager is %T, want the in-process *localruntime.InMemoryManager", m)
	}
	if limit := budget + time.Second; elapsed > limit {
		t.Fatalf("connectRuntimeManager took %s, want at most %s", elapsed, limit)
	}
	if launches.Load() == 0 {
		t.Fatal("model host was never launched")
	}

	// The abandoned client must not keep trying to launch a host.
	n := launches.Load()
	time.Sleep(3 * min(ensure, budget))
	if got := launches.Load(); got != n {
		t.Fatalf("launches went from %d to %d after fallback", n, got)
	}

	entries, err := crashlog.TailEntries(logPath, 50)
	if err != nil {
		t.Fatalf("TailEntries: %v", err)
	}
	for _, e := range entries {
		if e.Kind == crashlog.KindRuntimeEvent && e.Event == crashlog.EventModelHostUnavailable {
			return
		}
	}
	t.Fatalf("no %s event in crash log: %+v", crashlog.EventModelHostUnavailable, entries)
}

func TestConnectReturnsErrorWhenCallerCancelled(t *testing.T) {
	connectTestEnv(t)
	prev := launchModelHost
	launchModelHost = func(string) func(context.Context) error {
		return func(context.Context) error { return nil }
	}
	t.Cleanup(func() { launchModelHost = prev })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m, closeFn, err := connectRuntimeManager(ctx, ollamaConfig(), nil)
	if err == nil {
		if closeFn != nil {
			closeFn()
		}
		t.Fatalf("connectRuntimeManager with a cancelled ctx returned %T and no error, want an error", m)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if m != nil {
		t.Fatalf("manager = %T, want nil on error", m)
	}
}
