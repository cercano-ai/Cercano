package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"cercano/source/server/internal/filelock"
	"cercano/source/server/internal/hostregistry"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/paths"
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
			code := upgradeRestartCommand(tt.args, &output, upgradeRestartDeps{
				legacyPort: func() (string, error) {
					cfgCalls++
					if tt.cfgErr {
						return "", errors.New("config error")
					}
					return "4242", nil
				},
				executable: func() (string, error) {
					if tt.exeErr {
						return "", errors.New("exe error")
					}
					return "/fake/Cellar/cercano/2/bin/cercano", nil
				},
				profileAgents:     noProfileAgents,
				restartProfile:    unexpectedProfileRestart(t),
				relaunchModelHost: func(context.Context, string) (bool, error) { return false, nil },
				restartAddress: func(ctx context.Context, path string, endpoint netip.AddrPort) (bool, error) {
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
				},
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
	return upgradeRestartCommand(nil, out, upgradeRestartDeps{
		legacyPort:        func() (string, error) { return "4242", nil },
		executable:        func() (string, error) { return "/fake/bin/cercano", nil },
		profileAgents:     noProfileAgents,
		restartProfile:    unexpectedProfileRestart(t),
		restartAddress:    func(context.Context, string, netip.AddrPort) (bool, error) { return restarted, nil },
		relaunchModelHost: relaunch,
	})
}

func noProfileAgents() ([]profileAgent, error) { return nil, nil }

// unexpectedProfileRestart is a profile restart for tests in which no profile
// agent is running.
func unexpectedProfileRestart(t *testing.T) func(context.Context, string, profileAgent, netip.AddrPort) (bool, error) {
	return func(_ context.Context, _ string, a profileAgent, _ netip.AddrPort) (bool, error) {
		t.Errorf("unexpected restart of profile %q", a.Profile)
		return false, errors.New("unexpected profile restart")
	}
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
	upgradeRestartCommand([]string{"--timeout", "1ms"}, &out, upgradeRestartDeps{
		legacyPort:     func() (string, error) { return "4242", nil },
		executable:     func() (string, error) { return "/fake/bin/cercano", nil },
		profileAgents:  noProfileAgents,
		restartProfile: unexpectedProfileRestart(t),
		relaunchModelHost: func(ctx context.Context, _ string) (bool, error) {
			dl, ok := ctx.Deadline()
			if !ok {
				t.Fatal("no shutdown deadline")
			}
			got = time.Until(dl)
			return false, ctx.Err()
		},
		restartAddress: func(context.Context, string, netip.AddrPort) (bool, error) {
			time.Sleep(10 * time.Millisecond)
			return true, nil
		},
	})
	if got < time.Second {
		t.Fatalf("shutdown budget %s was derived from the spent restart deadline", got)
	}
}

// fakeProfileAgent is a running profile agent as profileAgents reports it,
// with its profile directory in a temporary location.
func fakeProfileAgent(t *testing.T, profile, addr string) profileAgent {
	t.Helper()
	return profileAgent{
		Profile: profile,
		Dir:     filepath.Join(t.TempDir(), profile),
		Record:  hostregistry.Record{Profile: profile, PID: 4242, Addr: addr, InstanceID: "inst-" + profile},
	}
}

// eventLog records the order of restart and relaunch calls.
type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(e string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
}

func (l *eventLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.events, ",")
}

// profileDeps runs the command against the given running profile agents.
// restart decides each profile's outcome; every call is logged as
// "restart:<profile>", the legacy-address restart as "legacy", and the model
// host relaunch as "modelhost".
func profileDeps(t *testing.T, log *eventLog, agents []profileAgent, restart func(profileAgent) (bool, error)) upgradeRestartDeps {
	return upgradeRestartDeps{
		legacyPort:    func() (string, error) { return "4242", nil },
		executable:    func() (string, error) { return "/fake/bin/cercano", nil },
		profileAgents: func() ([]profileAgent, error) { return agents, nil },
		restartProfile: func(ctx context.Context, exe string, a profileAgent, endpoint netip.AddrPort) (bool, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("no restart deadline")
			}
			if exe != "/fake/bin/cercano" || endpoint.String() != a.Record.Addr {
				t.Errorf("restart of %q targets %s with %s, want %s", a.Profile, endpoint, exe, a.Record.Addr)
			}
			log.add("restart:" + a.Profile)
			return restart(a)
		},
		restartAddress: func(_ context.Context, _ string, endpoint netip.AddrPort) (bool, error) {
			log.add("legacy")
			return true, nil
		},
		relaunchModelHost: func(context.Context, string) (bool, error) {
			log.add("modelhost")
			return true, nil
		},
	}
}

func restartOK(profileAgent) (bool, error) { return true, nil }

func writeAgentRecord(t *testing.T, rec hostregistry.Record) {
	t.Helper()
	if err := hostregistry.Write(paths.ProfileDir(rec.Profile), rec); err != nil {
		t.Fatal(err)
	}
}

// TestUpgradeRestartsEveryProfile: every profile whose recorded agent answers
// as itself is restarted once, at its recorded address. Profiles without a
// record, with a dead record, or whose recorded address answers as another
// agent are left alone, and the legacy address is not touched.
func TestUpgradeRestartsEveryProfile(t *testing.T) {
	tempHome(t)
	live := map[string]string{}
	for _, p := range []string{"default", "work"} {
		addr := serveHostIdentity(t, server.HostIdentity{Profile: p, PID: 100, InstanceID: "inst-" + p})
		writeAgentRecord(t, hostregistry.Record{Profile: p, PID: 100, Addr: addr, InstanceID: "inst-" + p})
		live[p] = addr
	}
	writeAgentRecord(t, hostregistry.Record{Profile: "stale", PID: 101, Addr: refusedAddr(t), InstanceID: "gone"})
	reused := serveHostIdentity(t, server.HostIdentity{Profile: "other", PID: 102, InstanceID: "inst-other"})
	writeAgentRecord(t, hostregistry.Record{Profile: "reused", PID: 103, Addr: reused, InstanceID: "inst-reused"})
	if err := os.MkdirAll(paths.ProfileDir("idle"), 0o700); err != nil {
		t.Fatal(err)
	}

	var log eventLog
	deps := profileDeps(t, &log, nil, restartOK)
	deps.profileAgents = runningProfileAgents
	var out bytes.Buffer
	code := upgradeRestartCommand(nil, &out, deps)
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if got, want := log.String(), "restart:default,restart:work,modelhost"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	for _, p := range []string{"default", "work"} {
		if want := fmt.Sprintf("Restarted agent for profile %q.", p); !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q: %s", want, out.String())
		}
	}
}

func TestUpgradeContinuesAfterOneFailure(t *testing.T) {
	agents := []profileAgent{
		fakeProfileAgent(t, "alpha", "127.0.0.1:5001"),
		fakeProfileAgent(t, "beta", "127.0.0.1:5002"),
		fakeProfileAgent(t, "gamma", "127.0.0.1:5003"),
	}
	var log eventLog
	var out bytes.Buffer
	code := upgradeRestartCommand(nil, &out, profileDeps(t, &log, agents, func(a profileAgent) (bool, error) {
		if a.Profile == "beta" {
			return false, errors.New("beta wedged")
		}
		return true, nil
	}))
	if code != 1 {
		t.Fatalf("code=%d, want 1; output=%s", code, out.String())
	}
	if got, want := log.String(), "restart:alpha,restart:beta,restart:gamma,modelhost"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	for _, want := range []string{
		`Restarted agent for profile "alpha".`,
		`Restarted agent for profile "gamma".`,
		`profile "beta"`, "beta wedged", "update may already be installed",
		"Shared model host restarted",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q: %s", want, out.String())
		}
	}
}

// When every attempted restart fails, the model host is left alone, as when
// the single legacy agent's restart fails.
func TestUpgradeAllProfilesFailingKeepsModelHost(t *testing.T) {
	agents := []profileAgent{fakeProfileAgent(t, "default", "127.0.0.1:5001"), fakeProfileAgent(t, "work", "127.0.0.1:5002")}
	var log eventLog
	var out bytes.Buffer
	code := upgradeRestartCommand(nil, &out, profileDeps(t, &log, agents, func(profileAgent) (bool, error) {
		return false, errors.New("refused")
	}))
	if code != 1 {
		t.Fatalf("code=%d, want 1; output=%s", code, out.String())
	}
	if got, want := log.String(), "restart:default,restart:work"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
}

// Each profile's launch lock is held while its agent restarts, so a client
// cannot launch a competing agent for that profile meanwhile, and released
// afterwards whether the restart succeeded or failed.
func TestUpgradeHoldsProfileLaunchLockDuringRestart(t *testing.T) {
	agents := []profileAgent{fakeProfileAgent(t, "default", "127.0.0.1:5001"), fakeProfileAgent(t, "work", "127.0.0.1:5002")}
	lockPath := func(a profileAgent) string { return filepath.Join(a.Dir, "launch.lock") }
	var log eventLog
	var out bytes.Buffer
	code := upgradeRestartCommand(nil, &out, profileDeps(t, &log, agents, func(a profileAgent) (bool, error) {
		for _, b := range agents {
			release, ok, err := filelock.TryLock(lockPath(b))
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				release()
			}
			if held := !ok; held != (b.Profile == a.Profile) {
				t.Errorf("restarting %q: launch lock of %q held=%v", a.Profile, b.Profile, held)
			}
		}
		if a.Profile == "work" {
			return false, errors.New("failed")
		}
		return true, nil
	}))
	if code != 1 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	for _, a := range agents {
		release, ok, err := filelock.TryLock(lockPath(a))
		if err != nil || !ok {
			t.Fatalf("launch lock of %q still held after the restart (err %v)", a.Profile, err)
		}
		release()
	}
}

// A client holding a profile's launch lock past the bounded wait fails that
// profile's restart without touching its agent; the others still restart.
func TestUpgradeGivesUpOnBusyLaunchLock(t *testing.T) {
	prev := upgradeLaunchLockWait
	upgradeLaunchLockWait = 200 * time.Millisecond
	t.Cleanup(func() { upgradeLaunchLockWait = prev })
	agents := []profileAgent{fakeProfileAgent(t, "default", "127.0.0.1:5001"), fakeProfileAgent(t, "work", "127.0.0.1:5002")}
	release, ok, err := filelock.TryLock(filepath.Join(agents[1].Dir, "launch.lock"))
	if err != nil || !ok {
		t.Fatalf("TryLock: %v %v", ok, err)
	}
	defer release()
	var log eventLog
	var out bytes.Buffer
	code := upgradeRestartCommand(nil, &out, profileDeps(t, &log, agents, restartOK))
	if code != 1 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if got, want := log.String(), "restart:default,modelhost"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	if !strings.Contains(out.String(), `profile "work"`) || !strings.Contains(out.String(), "still starting") {
		t.Fatalf("output does not name the failed profile and why: %s", out.String())
	}
}

// A launch lock that cannot be taken at all (not a busy one) is reported as
// such, without waiting and without touching the agent.
func TestUpgradeReportsUnusableLaunchLock(t *testing.T) {
	agents := []profileAgent{fakeProfileAgent(t, "work", "127.0.0.1:5002")}
	if err := os.WriteFile(agents[0].Dir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	var log eventLog
	var out bytes.Buffer
	if code := upgradeRestartCommand(nil, &out, profileDeps(t, &log, agents, restartOK)); code != 1 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if log.String() != "" {
		t.Fatalf("events = %s, want none", log.String())
	}
	if !strings.Contains(out.String(), "take the profile's launch lock") || strings.Contains(out.String(), "another client") {
		t.Fatalf("output: %s", out.String())
	}
}

// With no profile agent running, a pre-profile agent on the legacy address is
// restarted as before.
func TestUpgradeRestartsLegacyAgentWithoutProfileAgents(t *testing.T) {
	var log eventLog
	var out bytes.Buffer
	deps := profileDeps(t, &log, nil, restartOK)
	var target string
	deps.restartAddress = func(_ context.Context, _ string, endpoint netip.AddrPort) (bool, error) {
		target = endpoint.String()
		log.add("legacy")
		return true, nil
	}
	if code := upgradeRestartCommand(nil, &out, deps); code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if got, want := log.String(), "legacy,modelhost"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	if target != "127.0.0.1:4242" {
		t.Fatalf("legacy restart targeted %s, want 127.0.0.1:4242", target)
	}
	if !strings.Contains(out.String(), "Running agent restarted using the installed binary.") {
		t.Fatalf("output: %s", out.String())
	}
}

func TestUpgradeRelaunchesModelHostOnceAfterAllProfiles(t *testing.T) {
	agents := []profileAgent{
		fakeProfileAgent(t, "default", "127.0.0.1:5001"),
		fakeProfileAgent(t, "work", "127.0.0.1:5002"),
		fakeProfileAgent(t, "play", "127.0.0.1:5003"),
	}
	var log eventLog
	var out bytes.Buffer
	if code := upgradeRestartCommand(nil, &out, profileDeps(t, &log, agents, restartOK)); code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if got, want := log.String(), "restart:default,restart:work,restart:play,modelhost"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	if n := strings.Count(out.String(), "Shared model host restarted"); n != 1 {
		t.Fatalf("model host reported %d times: %s", n, out.String())
	}
}

// A default-profile agent recorded at the legacy address is restarted once,
// as a profile agent, whether or not this installation owns it.
func TestUpgradeDefaultProfileAtLegacyAddressRestartedOnce(t *testing.T) {
	for _, owned := range []bool{true, false} {
		agents := []profileAgent{fakeProfileAgent(t, "default", "127.0.0.1:4242")}
		var log eventLog
		var out bytes.Buffer
		code := upgradeRestartCommand(nil, &out, profileDeps(t, &log, agents, func(profileAgent) (bool, error) { return owned, nil }))
		if code != 0 {
			t.Fatalf("owned=%v: code=%d output=%s", owned, code, out.String())
		}
		if got, want := log.String(), "restart:default,modelhost"; got != want {
			t.Fatalf("owned=%v: events = %s, want %s", owned, got, want)
		}
		if !owned && !strings.Contains(out.String(), "nothing started") {
			t.Fatalf("owned=false: output: %s", out.String())
		}
	}
}

// A record naming a non-loopback address is reported, never restarted.
func TestUpgradeRefusesNonLoopbackRecord(t *testing.T) {
	agents := []profileAgent{fakeProfileAgent(t, "work", "192.0.2.1:9000"), fakeProfileAgent(t, "default", "127.0.0.1:5001")}
	var log eventLog
	var out bytes.Buffer
	code := upgradeRestartCommand(nil, &out, profileDeps(t, &log, agents, restartOK))
	if code != 1 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if got, want := log.String(), "restart:default,modelhost"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	if !strings.Contains(out.String(), `profile "work"`) {
		t.Fatalf("output: %s", out.String())
	}
}

// An explicit --address restarts only the agent there.
func TestUpgradeExplicitAddressSkipsProfiles(t *testing.T) {
	var log eventLog
	var out bytes.Buffer
	deps := profileDeps(t, &log, nil, restartOK)
	deps.profileAgents = func() ([]profileAgent, error) {
		t.Error("profiles listed despite --address")
		return nil, nil
	}
	if code := upgradeRestartCommand([]string{"--address", "127.0.0.1:4343"}, &out, deps); code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if got, want := log.String(), "legacy,modelhost"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
}

func TestUpgradeProfileListErrorRestartsNothing(t *testing.T) {
	var log eventLog
	var out bytes.Buffer
	deps := profileDeps(t, &log, nil, restartOK)
	deps.profileAgents = func() ([]profileAgent, error) { return nil, errors.New("unreadable") }
	if code := upgradeRestartCommand(nil, &out, deps); code != 1 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if log.String() != "" {
		t.Fatalf("events = %s, want none", log.String())
	}
}

// profileAgentRegistered reports a replacement ready only once the profile's
// record names that process and the agent there answers as the profile's
// agent with that process ID.
func TestProfileAgentRegistered(t *testing.T) {
	tempHome(t)
	dir := paths.ProfileDir("work")
	probe := profileAgentRegistered("work", dir)
	ctx := context.Background()

	writeAgentRecord(t, hostregistry.Record{Profile: "work", PID: 111, Addr: refusedAddr(t), InstanceID: "old"})
	if probe(ctx, 222) {
		t.Fatal("ready while the record still names the old agent")
	}
	newAddr := serveHostIdentity(t, server.HostIdentity{Profile: "work", PID: 222, InstanceID: "new"})
	writeAgentRecord(t, hostregistry.Record{Profile: "work", PID: 222, Addr: newAddr, InstanceID: "new"})
	if !probe(ctx, 222) {
		t.Fatal("not ready once the replacement registered and answers")
	}
	if probe(ctx, 333) {
		t.Fatal("ready for a process the record does not name")
	}
	// The record names the replacement, but the agent answering there is
	// another process.
	imposter := serveHostIdentity(t, server.HostIdentity{Profile: "work", PID: 999, InstanceID: "inst"})
	writeAgentRecord(t, hostregistry.Record{Profile: "work", PID: 444, Addr: imposter, InstanceID: "inst"})
	if probe(ctx, 444) {
		t.Fatal("ready although the answering agent reports another PID")
	}
}

// The identity probe stays within the caller's deadline even when the
// recorded address accepts connections but never answers.
func TestProfileAgentRegisteredHonorsDeadline(t *testing.T) {
	tempHome(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lis.Close() })
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { conn.Close() })
		}
	}()
	writeAgentRecord(t, hostregistry.Record{Profile: "work", PID: 555, Addr: lis.Addr().String(), InstanceID: "silent"})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if profileAgentRegistered("work", paths.ProfileDir("work"))(ctx, 555) {
		t.Fatal("ready although the agent never answered")
	}
	if elapsed := time.Since(start); elapsed > 600*time.Millisecond {
		t.Fatalf("probe took %s, past the caller's 100ms deadline", elapsed)
	}
}
