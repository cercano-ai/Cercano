package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// Run the existing full production fixture in a disposable process: a broken
// signal fallback cannot terminate the parent, and exit status is part of the
// contract. External engines/HTTP/credentials remain trapped by that fixture;
// only the idle callback and production disposal are deliberately NOT faked.
func TestProductionDefaultIdleSubprocess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestProductionSingleHostStartupShutdown$/^legacy-default-idle-shutdown$", "-test.v", "-test.timeout=30s")
	dir := t.TempDir()
	// Allowlist rather than inherit model endpoints, credentials, user config,
	// development generations, or other CERCANO_* authority from the caller.
	for _, key := range []string{"SystemRoot", "WINDIR"} {
		if value := os.Getenv(key); value != "" {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "TMPDIR", "TMP", "TEMP"} {
		cmd.Env = append(cmd.Env, key+"="+dir)
	}
	cmd.Env = append(cmd.Env, "CERCANO_DISABLE_CREDENTIALS=1", "CERCANO_TEST_DEFAULT_IDLE_CHILD=1")
	cmd.Dir = filepath.Clean(dir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated idle process did not exit successfully (deadline=%v): %v\n%s", ctx.Err(), err, output)
	}
	if !strings.Contains(string(output), "default idle callback: production disposal verified") {
		t.Fatalf("child did not verify production disposal:\n%s", output)
	}
	t.Logf("isolated production idle process exited successfully:\n%s", output)
}

// Context is queried only AFTER SubscribeEvents has registered its subscriber.
// This gives a deterministic reconnect barrier without polling or sleeps.
type idleReconnectStream struct {
	grpc.ServerStream
	ctx        context.Context
	registered chan struct{}
}

func (s idleReconnectStream) Context() context.Context    { close(s.registered); return s.ctx }
func (idleReconnectStream) Send(*proto.ClientEvent) error { panic("unexpected event") }

func checkDefaultProductionIdleDisposal(t *testing.T, srv *server.Server, cleanup *productionShutdown) {
	t.Helper()
	disconnectLastSubscriber(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registered, finished := make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- srv.SubscribeEvents(&proto.SubscribeEventsRequest{}, idleReconnectStream{ctx: ctx, registered: registered})
	}()
	select {
	case <-registered:
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect did not register")
	}
	// A bounded negative observation window exceeds the production 2s grace.
	// Registration, not elapsed time, synchronizes the reconnect above.
	select {
	case <-cleanup.done:
		t.Fatal("production disposed while reconnected within idle grace")
	case <-time.After(2500 * time.Millisecond):
	}
	if cleanup.requested.Load() {
		t.Fatal("production shutdown requested while reconnected")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("last disconnect: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("last subscriber did not disconnect")
	}
	// No explicit Shutdown/Request call here: only the DEFAULT installed idle
	// callback may initiate cleanup. Deferred cleanup in the fixture runs later.
	select {
	case <-cleanup.done:
	case <-time.After(5 * time.Second):
		t.Fatal("default idle callback did not finish production disposal")
	}
	if !cleanup.requested.Load() || !cleanup.Status().Complete {
		t.Fatalf("idle disposal incomplete: %+v", cleanup.Status())
	}
}
