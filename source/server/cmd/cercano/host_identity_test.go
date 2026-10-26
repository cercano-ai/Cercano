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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/hostregistry"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/testenv"
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
		flagOn                 bool
		profile, envPort, want string
	}{
		// Flag on: every host binds loopback only.
		{true, "default", "", "127.0.0.1:50999"},
		{true, "work", "", "127.0.0.1:0"},
		{true, "work", "50999", "127.0.0.1:50999"},
		{true, "default", "50999", "127.0.0.1:50999"},
		// Flag off (the default): the single host binds all interfaces at
		// the configured port, as the pre-profile agent did.
		{false, "default", "", ":50999"},
		{false, "default", "50999", ":50999"},
	} {
		t.Setenv(paths.PerProfileAgentsEnv, strconv.FormatBool(tc.flagOn))
		t.Setenv("CERCANO_PORT", tc.envPort)
		if got := agentBindAddr(tc.profile, cfg); got != tc.want {
			t.Errorf("agentBindAddr(%q, flagOn=%v, CERCANO_PORT=%q) = %q, want %q", tc.profile, tc.flagOn, tc.envPort, got, tc.want)
		}
	}
}

func TestAdvertiseAddr(t *testing.T) {
	for _, tc := range []struct{ addr, want string }{
		{"[::]:50052", "127.0.0.1:50052"},
		{":50052", "127.0.0.1:50052"},
		{"0.0.0.0:50052", "127.0.0.1:50052"},
		{"127.0.0.1:50052", "127.0.0.1:50052"},
		{"localhost:50052", "localhost:50052"},
		{"[::1]:50052", "[::1]:50052"},
		{"192.168.1.4:50052", "192.168.1.4:50052"},
	} {
		if got := advertiseAddr(tc.addr); got != tc.want {
			t.Errorf("advertiseAddr(%q) = %q, want %q", tc.addr, got, tc.want)
		}
	}
}

// A listener bound to all interfaces — the flag-off default-profile bind —
// advertises a dialable loopback address: the recorded host is loopback and
// a client can actually connect to it.
func TestAdvertiseAddrDialableForWildcardListener(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skipf("cannot bind all interfaces in this environment: %v", err)
	}
	defer lis.Close()
	adv := advertiseAddr(lis.Addr().String())
	host, port, err := net.SplitHostPort(adv)
	if err != nil {
		t.Fatalf("advertiseAddr(%q) = %q: %v", lis.Addr().String(), adv, err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("advertised host = %q, want 127.0.0.1", host)
	}
	_, boundPort, _ := net.SplitHostPort(lis.Addr().String())
	if port != boundPort {
		t.Fatalf("advertised port = %q, want the bound port %q", port, boundPort)
	}

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := lis.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	conn, err := net.Dial("tcp", adv)
	if err != nil {
		t.Fatalf("dial the advertised address %q: %v", adv, err)
	}
	conn.Close()
	select {
	case c := <-accepted:
		c.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("the dial to the advertised address was not accepted")
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

// withHandoverWait shortens how long a new agent waits for the profile lock.
func withHandoverWait(t *testing.T, d time.Duration) {
	t.Helper()
	prev := agentHandoverWait
	agentHandoverWait = d
	t.Cleanup(func() { agentHandoverWait = prev })
}

// serveHostIdentity serves an Agent that reports id on a loopback port and
// returns its address.
func serveHostIdentity(t *testing.T, id server.HostIdentity) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.NewServer(nil, nil, nil, nil, nil)
	srv.SetHostIdentity(id)
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, srv)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	return lis.Addr().String()
}

// refusedAddr is a loopback address nothing listens on, like a draining
// agent's address after GracefulStop has closed its listener.
func refusedAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	lis.Close()
	return addr
}

func TestAcquireAgentHostRefusesHealthyAgent(t *testing.T) {
	tempHome(t)
	withHandoverWait(t, 5*time.Second)
	first, err := acquireAgentHost("work")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.close()
	if err := first.register(serveHostIdentity(t, first.identity())); err != nil {
		t.Fatalf("register: %v", err)
	}
	start := time.Now()
	second, err := acquireAgentHost("work")
	if err == nil {
		second.close()
		t.Fatal("second acquire for the same profile succeeded")
	}
	want := fmt.Sprintf(`profile "work" already has a running agent (pid %d)`, os.Getpid())
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("refusing a healthy agent took %s; it must not wait for the lock", elapsed)
	}

	// A different profile is independent.
	other, err := acquireAgentHost("other")
	if err != nil {
		t.Fatalf("acquire other profile: %v", err)
	}
	other.close()
}

// A draining agent has closed its listener but still holds the lock until it
// exits; the new agent waits and then serves.
func TestAcquireAgentHostWaitsForDrainingAgent(t *testing.T) {
	home := tempHome(t)
	withHandoverWait(t, 10*time.Second)
	first, err := acquireAgentHost("work")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := first.register(refusedAddr(t)); err != nil {
		t.Fatalf("register: %v", err)
	}
	go func() {
		time.Sleep(500 * time.Millisecond)
		first.close()
	}()
	second, err := acquireAgentHost("work")
	if err != nil {
		t.Fatalf("acquire after the draining agent exited: %v", err)
	}
	defer second.close()
	if err := second.register("127.0.0.1:40002"); err != nil {
		t.Fatalf("register: %v", err)
	}
	rec, err := hostregistry.Read(filepath.Join(paths.ProfilesRootIn(home), "work"))
	if err != nil || rec.InstanceID != second.instanceID {
		t.Fatalf("record = %+v, %v; want the new agent's", rec, err)
	}
}

// An agent still starting holds the lock with no record yet; once it answers,
// the new agent reports it as running.
func TestAcquireAgentHostRefusesAgentThatFinishesStarting(t *testing.T) {
	tempHome(t)
	withHandoverWait(t, 10*time.Second)
	first, err := acquireAgentHost("work")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.close()
	addr := serveHostIdentity(t, first.identity())
	go func() {
		time.Sleep(500 * time.Millisecond)
		_ = first.register(addr)
	}()
	start := time.Now()
	second, err := acquireAgentHost("work")
	if err == nil {
		second.close()
		t.Fatal("second acquire succeeded while the first agent runs")
	}
	want := fmt.Sprintf(`profile "work" already has a running agent (pid %d)`, os.Getpid())
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("took %s to notice the started agent", elapsed)
	}
}

func TestAcquireAgentHostGivesUpOnAgentThatNeverExits(t *testing.T) {
	tempHome(t)
	withHandoverWait(t, 600*time.Millisecond)
	first, err := acquireAgentHost("work")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.close()
	if err := first.register(refusedAddr(t)); err != nil {
		t.Fatalf("register: %v", err)
	}
	start := time.Now()
	_, err = acquireAgentHost("work")
	if err == nil {
		t.Fatal("second acquire succeeded while the lock is held")
	}
	want := fmt.Sprintf(`the previous agent for profile "work" (pid %d) is still finishing its work; try again shortly`, os.Getpid())
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
	if elapsed := time.Since(start); elapsed < 600*time.Millisecond || elapsed > 5*time.Second {
		t.Fatalf("gave up after %s, want about the 600ms wait", elapsed)
	}
}

func TestAcquireAgentHostGivesUpWithoutRecordOmitsPID(t *testing.T) {
	tempHome(t)
	withHandoverWait(t, 300*time.Millisecond)
	first, err := acquireAgentHost("work")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer first.close()
	_, err = acquireAgentHost("work")
	if err == nil {
		t.Fatal("second acquire succeeded")
	}
	want := `the previous agent for profile "work" is still finishing its work; try again shortly`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
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

// TestMain keeps these tests' default Cercano paths under a temporary home.
func TestMain(m *testing.M) {
	code := testenv.IsolateHome(m)
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
// Cercano settings inherited from the developer's shell. These tests run
// several profile agents side by side, so they turn per-profile agent
// hosts on.
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
		"CERCANO_PER_PROFILE_AGENTS=1",
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
