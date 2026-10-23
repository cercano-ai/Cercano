package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/hostregistry"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestAgentBindAddr(t *testing.T) {
	cfg := config.Defaults()
	cfg.Port = "50999"
	for _, tc := range []struct {
		profile, envPort, want string
	}{
		{"default", "", "127.0.0.1:50999"},
		{"work", "", "127.0.0.1:0"},
		{"work", "50999", "127.0.0.1:50999"},
		{"default", "50999", "127.0.0.1:50999"},
	} {
		t.Setenv("CERCANO_PORT", tc.envPort)
		if got := agentBindAddr(tc.profile, cfg); got != tc.want {
			t.Errorf("agentBindAddr(%q, CERCANO_PORT=%q) = %q, want %q", tc.profile, tc.envPort, got, tc.want)
		}
	}
}

func TestEmbeddedGRPCBindsLoopback(t *testing.T) {
	host, _, err := net.SplitHostPort(embeddedGRPCBindAddr)
	if err != nil || host != "127.0.0.1" {
		t.Fatalf("embeddedGRPCBindAddr = %q, want a 127.0.0.1 address", embeddedGRPCBindAddr)
	}
}

func TestListenFailureNamesPort(t *testing.T) {
	err := listenFailure("127.0.0.1:50052", errors.New("bind: address already in use"))
	if !strings.Contains(err.Error(), "port 50052") {
		t.Fatalf("listenFailure = %q, want it to name port 50052", err)
	}
}

func TestModelHostRemoteOptionsReportProfile(t *testing.T) {
	tempHome(t)
	t.Setenv("CERCANO_PROFILE", "work")
	if got := modelHostRemoteOptions(t.TempDir()).Profile; got != "work" {
		t.Fatalf("RemoteOptions.Profile = %q, want work", got)
	}
}

func TestAcquireAgentHostRefusesHeldProfile(t *testing.T) {
	tempHome(t)
	first, err := acquireAgentHost("work")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.close()
	if err := first.register("127.0.0.1:40000"); err != nil {
		t.Fatalf("register: %v", err)
	}
	second, err := acquireAgentHost("work")
	if err == nil {
		second.close()
		t.Fatal("second acquire for the same profile succeeded")
	}
	want := fmt.Sprintf(`profile "work" already has a running agent (pid %d)`, os.Getpid())
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}

	// A different profile is independent.
	other, err := acquireAgentHost("other")
	if err != nil {
		t.Fatalf("acquire other profile: %v", err)
	}
	other.close()
}

func TestAcquireAgentHostWithoutRecordOmitsPID(t *testing.T) {
	tempHome(t)
	first, err := acquireAgentHost("work")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.close()
	_, err = acquireAgentHost("work")
	if err == nil {
		t.Fatal("second acquire succeeded")
	}
	if !strings.HasPrefix(err.Error(), `profile "work" already has a running agent`) || strings.Contains(err.Error(), "pid") {
		t.Fatalf("error = %q, want the running-agent message without a PID", err)
	}
}

func TestAgentHostCloseRemovesOwnRecordThenReleasesLock(t *testing.T) {
	home := tempHome(t)
	h, err := acquireAgentHost("work")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.register("127.0.0.1:40001"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(paths.ProfilesRootIn(home), "work")
	rec, err := hostregistry.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Profile != "work" || rec.PID != os.Getpid() || rec.Addr != "127.0.0.1:40001" || rec.InstanceID == "" || rec.Version != version || rec.StartedAt.IsZero() {
		t.Fatalf("record = %+v", rec)
	}
	h.close()
	h.close() // idempotent
	if _, err := hostregistry.Read(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("record after close: err = %v, want ErrNotExist", err)
	}
	again, err := acquireAgentHost("work")
	if err != nil {
		t.Fatalf("lock not released by close: %v", err)
	}
	again.close()
}

func TestAgentHostClearsStaleRecordOnAcquire(t *testing.T) {
	home := tempHome(t)
	dir := filepath.Join(paths.ProfilesRootIn(home), "work")
	stale := hostregistry.Record{Profile: "work", PID: 999999, Addr: "127.0.0.1:1", InstanceID: "stale"}
	if err := hostregistry.Write(dir, stale); err != nil {
		t.Fatal(err)
	}
	h, err := acquireAgentHost("work")
	if err != nil {
		t.Fatal(err)
	}
	defer h.close()
	if _, err := hostregistry.Read(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale record left in place after acquiring the lock: %v", err)
	}
	if h.instanceID == "" || h.instanceID == "stale" {
		t.Fatalf("instanceID = %q, want a fresh random ID", h.instanceID)
	}
}

// ---- end-to-end: real agent processes ----

var agentBin struct {
	once sync.Once
	dir  string
	path string
	err  error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if agentBin.dir != "" {
		_ = os.RemoveAll(agentBin.dir)
	}
	os.Exit(code)
}

// buildAgentBinary builds this command once per test run.
func buildAgentBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the cercano binary")
	}
	agentBin.once.Do(func() {
		dir, err := os.MkdirTemp("", "cercano-agent-bin-")
		if err != nil {
			agentBin.err = err
			return
		}
		agentBin.dir = dir
		name := "cercano"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		agentBin.path = filepath.Join(dir, name)
		out, err := exec.Command("go", "build", "-o", agentBin.path, ".").CombinedOutput()
		if err != nil {
			agentBin.err = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if agentBin.err != nil {
		t.Fatal(agentBin.err)
	}
	return agentBin.path
}

// agentEnv is the environment for an agent under test: a private home, the
// given profile, in-process local models, and no cloud credentials or
// Cercano settings inherited from the developer's shell.
func agentEnv(home, profile string) []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		switch {
		case upper == "HOME", upper == "USERPROFILE", upper == "XDG_CONFIG_HOME",
			strings.HasPrefix(upper, "CERCANO_"),
			strings.Contains(upper, "API_KEY"), strings.Contains(upper, "TOKEN"),
			strings.Contains(upper, "SECRET"):
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"HOME="+home,
		"USERPROFILE="+home,
		"CERCANO_PROFILE="+profile,
		"CERCANO_MODEL_HOST=inprocess",
	)
}

type runningAgent struct {
	cmd    *exec.Cmd
	out    *bytes.Buffer
	done   chan error
	record hostregistry.Record
}

// startAgent runs `cercano agent` for profile and waits for its registry
// record. The agent is stopped when the test ends.
func startAgent(t *testing.T, bin, home, profile string) *runningAgent {
	t.Helper()
	cmd := exec.Command(bin, "agent")
	cmd.Env = agentEnv(home, profile)
	out := &bytes.Buffer{}
	cmd.Stdout = &lockedWriter{w: out}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatalf("start agent: %v", err)
	}
	a := &runningAgent{cmd: cmd, out: out, done: make(chan error, 1)}
	go func() { a.done <- cmd.Wait() }()
	t.Cleanup(func() { stopAgent(t, a) })

	dir := filepath.Join(paths.ProfilesRootIn(home), profile)
	deadline := time.Now().Add(90 * time.Second)
	for {
		rec, err := hostregistry.Read(dir)
		if err == nil && rec.PID == cmd.Process.Pid {
			a.record = rec
			return a
		}
		select {
		case err := <-a.done:
			a.done <- err
			t.Fatalf("agent for profile %q exited before registering: %v\n%s", profile, err, a.output())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("agent for profile %q did not register within 90s\n%s", profile, a.output())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (a *runningAgent) output() string {
	if lw, ok := a.cmd.Stdout.(*lockedWriter); ok {
		return lw.String()
	}
	return ""
}

// stopAgent asks the agent to shut down and waits for it to exit.
func stopAgent(t *testing.T, a *runningAgent) error {
	t.Helper()
	select {
	case err := <-a.done:
		a.done <- err
		return err
	default:
	}
	if runtime.GOOS == "windows" {
		_ = a.cmd.Process.Kill()
	} else {
		_ = a.cmd.Process.Signal(os.Interrupt)
	}
	select {
	case err := <-a.done:
		a.done <- err
		return err
	case <-time.After(30 * time.Second):
		_ = a.cmd.Process.Kill()
		err := <-a.done
		a.done <- err
		t.Errorf("agent pid %d did not exit within 30s of an interrupt", a.cmd.Process.Pid)
		return err
	}
}

type lockedWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (l *lockedWriter) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.String()
}

func hostIdentity(t *testing.T, addr string) *proto.HostIdentity {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := proto.NewAgentClient(conn).GetHostIdentity(ctx, &proto.GetHostIdentityRequest{})
	if err != nil {
		t.Fatalf("GetHostIdentity at %s: %v", addr, err)
	}
	return id
}

func TestSecondAgentForSameProfileExits(t *testing.T) {
	bin := buildAgentBinary(t)
	home := t.TempDir()
	first := startAgent(t, bin, home, "work")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	second := exec.CommandContext(ctx, bin, "agent")
	second.Env = agentEnv(home, "work")
	out, err := second.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("second agent: err = %v, want exit code 1\n%s", err, out)
	}
	want := fmt.Sprintf(`profile "work" already has a running agent (pid %d)`, first.cmd.Process.Pid)
	if !strings.Contains(string(out), want) {
		t.Fatalf("second agent output does not contain %q:\n%s", want, out)
	}

	// The refused agent must leave the running agent's record alone.
	rec, err := hostregistry.Read(filepath.Join(paths.ProfilesRootIn(home), "work"))
	if err != nil {
		t.Fatalf("record after refused start: %v", err)
	}
	if rec.InstanceID != first.record.InstanceID {
		t.Fatalf("record instance = %q, want the first agent's %q", rec.InstanceID, first.record.InstanceID)
	}
}

func TestAgentsForDifferentProfilesCoexist(t *testing.T) {
	bin := buildAgentBinary(t)
	home := t.TempDir()
	a := startAgent(t, bin, home, "a")
	b := startAgent(t, bin, home, "b")

	if a.record.Addr == b.record.Addr {
		t.Fatalf("both agents registered %s", a.record.Addr)
	}
	for _, tc := range []struct {
		profile string
		agent   *runningAgent
	}{{"a", a}, {"b", b}} {
		rec := tc.agent.record
		if rec.Profile != tc.profile || rec.PID != tc.agent.cmd.Process.Pid || rec.InstanceID == "" {
			t.Errorf("profile %s record = %+v", tc.profile, rec)
		}
		id := hostIdentity(t, rec.Addr)
		if id.GetProfile() != tc.profile || int(id.GetPid()) != rec.PID || id.GetInstanceId() != rec.InstanceID || id.GetVersion() != rec.Version {
			t.Errorf("GetHostIdentity at %s = %+v, want it to match record %+v", rec.Addr, id, rec)
		}
	}
	if a.record.InstanceID == b.record.InstanceID {
		t.Fatalf("both agents use instance ID %q", a.record.InstanceID)
	}
}

func TestAgentBindsLoopbackOnly(t *testing.T) {
	bin := buildAgentBinary(t)
	home := t.TempDir()
	a := startAgent(t, bin, home, "loop")
	host, port, err := net.SplitHostPort(a.record.Addr)
	if err != nil {
		t.Fatalf("registered address %q: %v", a.record.Addr, err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("registered host = %q, want 127.0.0.1", host)
	}
	if port == "0" || port == "" {
		t.Fatalf("registered port = %q, want the bound port", port)
	}
}

func TestAgentRemovesRecordOnShutdown(t *testing.T) {
	bin := buildAgentBinary(t)
	home := t.TempDir()
	a := startAgent(t, bin, home, "bye")
	if runtime.GOOS == "windows" {
		t.Skip("no graceful interrupt for a child process on Windows")
	}
	if err := stopAgent(t, a); err != nil {
		t.Fatalf("agent exit after interrupt: %v\n%s", err, a.output())
	}
	dir := filepath.Join(paths.ProfilesRootIn(home), "bye")
	if _, err := hostregistry.Read(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("agent.json after shutdown: err = %v, want ErrNotExist\n%s", err, a.output())
	}
	// The lock is free again: a new agent for the profile starts.
	startAgent(t, bin, home, "bye")
}
