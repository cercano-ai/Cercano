package main

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/filelock"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

func testHostOptions(t *testing.T, idle time.Duration) modelHostOptions {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	mgr := localruntime.NewManager()
	mgr.RegisterProvider(localruntimetest.NewFakeProvider("fake", localruntime.ModelRecord{ID: "m", Runtime: "fake"}))
	return modelHostOptions{
		RunDir:      filepath.Join(t.TempDir(), "run"),
		Config:      config.Defaults(),
		IdleTimeout: idle,
		Manager:     mgr,
	}
}

func startHost(t *testing.T, ctx context.Context, opts modelHostOptions) chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- runModelHost(ctx, opts) }()
	return done
}

func waitRecord(t *testing.T, dir string, d time.Duration) modelhost.Record {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if r, err := modelhost.ReadRecord(dir); err == nil {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("record did not appear")
	return modelhost.Record{}
}

func waitDone(t *testing.T, done chan error, d time.Duration) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatal("runModelHost did not return")
		return nil
	}
}

func hello(t *testing.T, rec modelhost.Record) *proto.HelloResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := modelhost.Dial(ctx, rec)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	resp, err := proto.NewModelHostClient(conn).Hello(ctx, &proto.HelloRequest{})
	if err != nil {
		t.Fatalf("Hello: %v", err)
	}
	return resp
}

func TestModelHostWritesRecordAndServes(t *testing.T) {
	opts := testHostOptions(t, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	done := startHost(t, ctx, opts)
	rec := waitRecord(t, opts.RunDir, 3*time.Second)
	host, _, err := net.SplitHostPort(rec.Addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		t.Fatalf("record addr %q is not an IP-literal loopback", rec.Addr)
	}
	if len(rec.Token) != 64 {
		t.Fatalf("token length = %d, want 64 hex chars", len(rec.Token))
	}
	if got := hello(t, rec).GetApiVersion(); got != modelhost.APIVersion {
		t.Fatalf("api version = %d, want %d", got, modelhost.APIVersion)
	}
	cancel()
	if err := waitDone(t, done, 5*time.Second); err != nil {
		t.Fatalf("runModelHost: %v", err)
	}
	if _, err := modelhost.ReadRecord(opts.RunDir); err == nil {
		t.Fatal("record still present after exit")
	}
}

func TestModelHostExitsWhenIdle(t *testing.T) {
	opts := testHostOptions(t, 200*time.Millisecond)
	done := startHost(t, context.Background(), opts)
	if err := waitDone(t, done, 2*time.Second); err != nil {
		t.Fatalf("runModelHost: %v", err)
	}
	if _, err := modelhost.ReadRecord(opts.RunDir); err == nil {
		t.Fatal("record still present after idle exit")
	}
}

func TestModelHostExitsOnShutdownRPC(t *testing.T) {
	opts := testHostOptions(t, time.Minute)
	done := startHost(t, context.Background(), opts)
	rec := waitRecord(t, opts.RunDir, 3*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := modelhost.Dial(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := proto.NewModelHostClient(conn).Shutdown(ctx, &proto.ShutdownRequest{}); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitDone(t, done, 5*time.Second); err != nil {
		t.Fatalf("runModelHost: %v", err)
	}
}

func TestSecondModelHostExitsQuietly(t *testing.T) {
	opts := testHostOptions(t, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := startHost(t, ctx, opts)
	rec := waitRecord(t, opts.RunDir, 3*time.Second)

	second := opts
	second.Manager = nil // must not build a manager at all
	if err := waitDone(t, startHost(t, context.Background(), second), 3*time.Second); err != nil {
		t.Fatalf("second host: %v", err)
	}
	hello(t, rec) // the first keeps serving
	cancel()
	waitDone(t, first, 5*time.Second)
}

func TestModelHostWaitsForDrainingHostToExit(t *testing.T) {
	opts := testHostOptions(t, time.Minute)
	dir := opts.RunDir
	// A previous host: holds the lock, left a record nobody answers.
	stale := modelhost.Record{PID: 1, Addr: "127.0.0.1:1", Token: "x", APIVersion: modelhost.APIVersion, StartedAt: time.Now()}
	if err := modelhost.WriteRecord(dir, stale); err != nil {
		t.Fatal(err)
	}
	release, ok, err := filelock.TryLock(filepath.Join(dir, "modelhost.lock"))
	if err != nil || !ok {
		t.Fatalf("TryLock: ok=%v err=%v", ok, err)
	}
	old := modelHostHandoverWait
	modelHostHandoverWait = 5 * time.Second
	defer func() { modelHostHandoverWait = old }()

	ctx, cancel := context.WithCancel(context.Background())
	done := startHost(t, ctx, opts)
	select {
	case err := <-done:
		t.Fatalf("returned while old host held the lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	release()
	deadline := time.Now().Add(3 * time.Second)
	var rec modelhost.Record
	for time.Now().Before(deadline) {
		if r, err := modelhost.ReadRecord(dir); err == nil && r.Addr != stale.Addr {
			rec = r
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rec.Addr == "" {
		t.Fatal("new host never wrote its record")
	}
	hello(t, rec)
	cancel()
	waitDone(t, done, 5*time.Second)
}

func TestModelHostHandoverTimesOut(t *testing.T) {
	opts := testHostOptions(t, time.Minute)
	if err := modelhost.WriteRecord(opts.RunDir, modelhost.Record{PID: 1, Addr: "127.0.0.1:1", Token: "x", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	release, ok, err := filelock.TryLock(filepath.Join(opts.RunDir, "modelhost.lock"))
	if err != nil || !ok {
		t.Fatal(err)
	}
	defer release()
	old := modelHostHandoverWait
	modelHostHandoverWait = 200 * time.Millisecond
	defer func() { modelHostHandoverWait = old }()
	err = waitDone(t, startHost(t, context.Background(), opts), 3*time.Second)
	if err == nil || errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "model host") {
		t.Fatalf("err = %v, want a model host handover error", err)
	}
}

func TestSecondModelHostExitsQuietlyWhileFirstIsStarting(t *testing.T) {
	opts := testHostOptions(t, time.Minute)
	dir := opts.RunDir
	if _, err := modelhost.ReadRecord(dir); err == nil {
		t.Fatal("unexpected record")
	}
	release, ok, err := filelock.TryLock(filepath.Join(dir, "modelhost.lock"))
	if err != nil || !ok {
		t.Fatalf("TryLock: ok=%v err=%v", ok, err)
	}
	defer release()
	old := modelHostHandoverWait
	modelHostHandoverWait = 10 * time.Second
	defer func() { modelHostHandoverWait = old }()

	// The "first host": holds the lock, and only after a slow start does its
	// record appear and its server answer.
	const token = "starting-host-token"
	srv := modelhost.NewServer(localruntime.NewManager(), modelhost.ServerOptions{Token: token})
	g := grpc.NewServer()
	srv.Register(g)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go g.Serve(lis)
	defer g.Stop()
	rec := modelhost.Record{PID: 1, Addr: lis.Addr().String(), Token: token, APIVersion: modelhost.APIVersion, StartedAt: time.Now()}
	time.AfterFunc(300*time.Millisecond, func() { _ = modelhost.WriteRecord(dir, rec) })

	start := time.Now()
	second := opts
	second.Manager = nil // must never build a manager or serve
	if err := waitDone(t, startHost(t, context.Background(), second), 5*time.Second); err != nil {
		t.Fatalf("second host: %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("second host took %s", d)
	}
	got, err := modelhost.ReadRecord(dir)
	if err != nil || got.Addr != rec.Addr {
		t.Fatalf("record changed: %+v, %v", got, err)
	}
}

func TestModelHostConfigIgnoresProfileConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CERCANO_PROFILE", "")
	if err := os.MkdirAll(filepath.Dir(config.DefaultPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	// machine.yaml is absent; the profile file carries a legacy runtime
	// section and a profile-only field. The host must see neither.
	body := "port: \"59999\"\nllama_server:\n  extra_args: [\"--from-profile\"]\n"
	if err := os.WriteFile(config.DefaultPath(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := machineRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	def := config.Defaults()
	if len(cfg.LlamaServer.ExtraArgs) != len(def.LlamaServer.ExtraArgs) {
		t.Errorf("ExtraArgs = %v, want defaults %v", cfg.LlamaServer.ExtraArgs, def.LlamaServer.ExtraArgs)
	}
	if cfg.Port != def.Port {
		t.Errorf("Port = %q, want default %q", cfg.Port, def.Port)
	}

	// With machine.yaml present, its sections are used.
	mach := config.MachineConfig{LlamaServer: def.LlamaServer, MistralRS: def.MistralRS}
	mach.LlamaServer.ExtraArgs = []string{"--from-machine"}
	if err := config.SaveMachine(mach); err != nil {
		t.Fatal(err)
	}
	cfg, err = machineRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.LlamaServer.ExtraArgs; len(got) != 1 || got[0] != "--from-machine" {
		t.Errorf("ExtraArgs = %v, want [--from-machine]", got)
	}
}
