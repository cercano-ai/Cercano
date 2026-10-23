package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"cercano/source/server/internal/filelock"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/internal/modelhost"
)

func TestUpgradeRestartCommand(t *testing.T) {
	for _, tt := range []struct {
		name                       string
		args                       []string
		cfgErr, exeErr, restartErr bool
		restarted                  bool
		wantCode, wantCalls        int
		message                    string
	}{
		{name: "help", args: []string{"--help"}, wantCode: 0, message: "Usage:"},
		{name: "invalid port", args: []string{"--address", "127.0.0.1:0"}, wantCode: 2},
		{name: "remote endpoint", args: []string{"--address", "192.0.2.1:9000"}, wantCode: 2},
		{name: "unbounded timeout", args: []string{"--timeout", "0s"}, wantCode: 2},
		{name: "extra arguments", args: []string{"surprise"}, wantCode: 2},
		{name: "configuration error", cfgErr: true, wantCode: 1},
		{name: "executable error", exeErr: true, wantCode: 1},
		{name: "absent", wantCode: 0, wantCalls: 1, message: "nothing started"},
		{name: "restarted", restarted: true, wantCode: 0, wantCalls: 1, message: "restarted using"},
		{name: "restart failure", restartErr: true, wantCode: 1, wantCalls: 1, message: "update may already be installed"},
		{name: "explicit address bypasses config", args: []string{"--address", "127.0.0.1:4242"}, cfgErr: true, wantCode: 0, wantCalls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			calls := 0
			cfgCalls := 0
			code := upgradeRestartCommand(tt.args, &output,
				func() (string, error) {
					cfgCalls++
					if tt.cfgErr {
						return "", errors.New("config error")
					}
					return "4242", nil
				},
				func() (string, error) {
					if tt.exeErr {
						return "", errors.New("exe error")
					}
					return "/fake/Cellar/cercano/2/bin/cercano", nil
				},
				func(context.Context, string) (bool, error) { return false, nil },
				func(ctx context.Context, path string, endpoint netip.AddrPort) (bool, error) {
					calls++
					if _, ok := ctx.Deadline(); !ok {
						t.Fatal("no restart deadline")
					}
					if path != "/fake/Cellar/cercano/2/bin/cercano" || endpoint.String() != "127.0.0.1:4242" {
						t.Fatal("incorrect restart target")
					}
					if tt.restartErr {
						return false, errors.New("failure")
					}
					return tt.restarted, nil
				})
			if code != tt.wantCode || calls != tt.wantCalls {
				t.Fatalf("code=%d calls=%d output=%s", code, calls, output.String())
			}
			if !strings.Contains(output.String(), tt.message) {
				t.Fatalf("missing output %q: %s", tt.message, output.String())
			}
			if (tt.name == "help" || tt.name == "explicit address bypasses config") && cfgCalls != 0 {
				t.Fatal("unexpected config access")
			}
		})
	}
}

func upgradeArgs(t *testing.T, out *bytes.Buffer, relaunch func(context.Context, string) (bool, error), restarted bool) int {
	t.Helper()
	return upgradeRestartCommand(nil, out,
		func() (string, error) { return "4242", nil },
		func() (string, error) { return "/fake/bin/cercano", nil },
		relaunch,
		func(context.Context, string, netip.AddrPort) (bool, error) { return restarted, nil })
}

// failLaunch is a launcher for tests in which no model host may be launched.
func failLaunch(t *testing.T) func(string) func(context.Context) error {
	return func(string) func(context.Context) error {
		return func(context.Context) error {
			t.Error("model host launched")
			return errors.New("unexpected launch")
		}
	}
}

// TestUpgradeRestartRelaunchesModelHostUnderLock: restart-after-upgrade
// replaces the running model host with one launched from its own (installed)
// executable while holding the launch lock, so an agent recovering from the
// shutdown waits for the lock and then attaches to the new host instead of
// launching its own, possibly older, binary.
func TestUpgradeRestartRelaunchesModelHostUnderLock(t *testing.T) {
	opts := testHostOptions(t, time.Minute)
	oldDone := startHost(t, context.Background(), opts)
	oldRec := waitRecord(t, opts.RunDir, 3*time.Second)

	newOpts := opts
	newMgr := localruntime.NewManager()
	newMgr.RegisterProvider(localruntimetest.NewFakeProvider("fake", localruntime.ModelRecord{ID: "m", Runtime: "fake"}))
	newOpts.Manager = newMgr
	newCtx, stopNew := context.WithCancel(context.Background())
	var newDone chan error
	t.Cleanup(func() {
		stopNew()
		if newDone != nil {
			<-newDone
		}
	})

	var launchedPath string
	var lockHeld bool
	var agentLaunches atomic.Int32
	agentDone := make(chan modelhost.Record, 1)
	launcher := func(path string) func(context.Context) error {
		return func(context.Context) error {
			launchedPath = path
			// The launch lock is held for the whole replacement.
			if release, ok, err := filelock.TryLock(filepath.Join(opts.RunDir, "launch.lock")); err == nil && ok {
				release()
			} else {
				lockHeld = err == nil
			}
			// An agent whose stream broke when the old host shut down
			// recovers now: it must not launch a host of its own.
			go func() {
				rec, err := modelhost.Ensure(context.Background(), modelhost.EnsureOptions{
					RunDir:  opts.RunDir,
					Timeout: 5 * time.Second,
					Launch:  func(context.Context) error { agentLaunches.Add(1); return nil },
				})
				if err != nil {
					t.Errorf("agent Ensure: %v", err)
				}
				agentDone <- rec
			}()
			time.Sleep(100 * time.Millisecond)
			newDone = startHost(t, newCtx, newOpts)
			return nil
		}
	}

	var out bytes.Buffer
	if code := upgradeArgs(t, &out, relaunchModelHostIn(opts.RunDir, launcher), true); code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if err := waitDone(t, oldDone, 5*time.Second); err != nil {
		t.Fatalf("old runModelHost: %v", err)
	}
	if launchedPath != "/fake/bin/cercano" {
		t.Fatalf("launched %q, want the installed executable", launchedPath)
	}
	if !lockHeld {
		t.Fatal("launch lock was not held while the new host launched")
	}
	newRec, err := modelhost.ReadRecord(opts.RunDir)
	if err != nil || newRec.Addr == oldRec.Addr {
		t.Fatalf("record after relaunch = %+v, %v; want the new host", newRec, err)
	}
	// Relaunch returned only once the new host answers.
	hello(t, newRec)
	select {
	case rec := <-agentDone:
		if rec.Addr != newRec.Addr {
			t.Fatalf("agent attached to %s, want the new host %s", rec.Addr, newRec.Addr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("agent Ensure did not return")
	}
	if n := agentLaunches.Load(); n != 0 {
		t.Fatalf("agent launched %d host(s) during the upgrade relaunch", n)
	}
	if !strings.Contains(out.String(), "model host restarted") {
		t.Fatalf("output does not report the model host restart: %s", out.String())
	}
}

func TestUpgradeRestartWithoutModelHost(t *testing.T) {
	var out bytes.Buffer
	dir := filepath.Join(t.TempDir(), "run")
	if code := upgradeArgs(t, &out, relaunchModelHostIn(dir, failLaunch(t)), true); code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if strings.Contains(out.String(), "model host") {
		t.Fatalf("unexpected model host message: %s", out.String())
	}
}

func TestUpgradeRestartWithStaleModelHostRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	lis.Close()
	if err := modelhost.WriteRecord(dir, modelhost.Record{PID: os.Getpid(), Addr: addr, Token: "tok", APIVersion: modelhost.APIVersion, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := upgradeArgs(t, &out, relaunchModelHostIn(dir, failLaunch(t)), true); code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
}

func TestUpgradeRestartModelHostFailureKeepsExitCode(t *testing.T) {
	var out bytes.Buffer
	code := upgradeArgs(t, &out, func(context.Context, string) (bool, error) { return false, errors.New("boom") }, true)
	if code != 0 || !strings.Contains(out.String(), "model host") {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
}

func TestUpgradeRestartRelaunchesModelHostWhenNoAgentRunning(t *testing.T) {
	called := 0
	var out bytes.Buffer
	if code := upgradeArgs(t, &out, func(context.Context, string) (bool, error) { called++; return false, nil }, false); code != 0 || called != 1 {
		t.Fatalf("code=%d called=%d", code, called)
	}
}

// wedgedManager lets Hello succeed but blocks Shutdown (StopAll lists
// instances first) until the caller's context ends.
type wedgedManager struct{ localruntime.Manager }

func (wedgedManager) Instances(ctx context.Context) ([]localruntime.InstanceRecord, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestUpgradeRestartBoundsWedgedModelHostShutdown(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	modelhost.NewServer(wedgedManager{localruntime.NewManager()}, modelhost.ServerOptions{Token: "tok"}).Register(g)
	go g.Serve(lis)
	t.Cleanup(g.Stop)
	if err := modelhost.WriteRecord(dir, modelhost.Record{PID: os.Getpid(), Addr: lis.Addr().String(), Token: "tok", APIVersion: modelhost.APIVersion, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	old := modelHostShutdownTimeout
	modelHostShutdownTimeout = 300 * time.Millisecond
	t.Cleanup(func() { modelHostShutdownTimeout = old })

	var out bytes.Buffer
	start := time.Now()
	code := upgradeArgs(t, &out, relaunchModelHostIn(dir, failLaunch(t)), true)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("shutdown took %s, want it bounded by %s", elapsed, modelHostShutdownTimeout)
	}
	if code != 0 || !strings.Contains(out.String(), "Could not restart the shared model host") {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
}

func TestUpgradeRestartShutdownBudgetIndependentOfRestartDeadline(t *testing.T) {
	var got time.Duration
	var out bytes.Buffer
	upgradeRestartCommand([]string{"--timeout", "1ms"}, &out,
		func() (string, error) { return "4242", nil },
		func() (string, error) { return "/fake/bin/cercano", nil },
		func(ctx context.Context, _ string) (bool, error) {
			dl, ok := ctx.Deadline()
			if !ok {
				t.Fatal("no shutdown deadline")
			}
			got = time.Until(dl)
			return false, ctx.Err()
		},
		func(context.Context, string, netip.AddrPort) (bool, error) {
			time.Sleep(10 * time.Millisecond)
			return true, nil
		})
	if got < time.Second {
		t.Fatalf("shutdown budget %s was derived from the spent restart deadline", got)
	}
}
