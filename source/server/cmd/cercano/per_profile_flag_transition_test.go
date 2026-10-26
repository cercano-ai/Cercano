//go:build linux

package main

// Transition tests for CERCANO_PER_PROFILE_AGENTS across
// restart-after-upgrade, with real agent processes from this build and from
// f79bf87a (per-profile agent hosts before the flag existed).
//
// These are integration tests with an EMULATED coordinator, not end-to-end
// tests of the Homebrew path. restart-after-upgrade replaces agents only on
// macOS (brewrestart's coordinator_darwin.go); on Linux brewrestart reports
// it unsupported. Here the real upgradeRestartCommand runs — agent discovery
// through agent.json and GetHostIdentity, --address matching, the profile
// launch lock, the production replacement check, readiness through
// profileAgentRegistered — with linuxReplay in place of the Darwin
// coordinator. In coordinateRestart's order, linuxReplay finds the process
// holding the endpoint, captures its argv, environment and working directory
// (from /proc rather than KERN_PROCARGS2), applies the command's replacement
// check to that environment with brewrestart.CheckReplacement (the call
// coordinateRestart makes after preflight), and only then asks it to shut
// down with ShutdownAgent, waits for it to exit, starts the installed
// executable with the captured launch state and waits for readiness, failing
// at once if the replacement exits. It omits the Darwin ownership checks
// (Homebrew keg, executable identity), preflight and the auto-launch lock,
// and acts only on processes the test itself started.
//
// The historical binary comes from test/e2e/scripts/build-historical.sh;
// set CERCANO_HISTORICAL_BIN_DIR to another directory of builds, and
// CERCANO_REQUIRE_HISTORICAL=1 to fail instead of skipping without it.

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/internal/hostregistry"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/paths"
)

// flagState is CERCANO_PER_PROFILE_AGENTS for one process: unset, or a value.
type flagState struct {
	set   bool
	value string
}

var (
	flagAbsent = flagState{}
	flagOn     = flagState{set: true, value: "1"}
)

func (f flagState) String() string {
	if !f.set {
		return "absent"
	}
	return strconv.Quote(f.value)
}

// historicalAgentBinary returns cercano built from rev.
func historicalAgentBinary(t *testing.T, rev string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("runs historical cercano binaries")
	}
	dir := os.Getenv("CERCANO_HISTORICAL_BIN_DIR")
	if dir == "" {
		dir = filepath.Join("..", "..", "..", "..", "test", "e2e", ".bin")
	}
	path, err := filepath.Abs(filepath.Join(dir, rev, "cercano"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		msg := fmt.Sprintf("%s is missing; run test/e2e/scripts/build-historical.sh", path)
		if os.Getenv("CERCANO_REQUIRE_HISTORICAL") == "1" {
			t.Fatal(msg)
		}
		t.Skip("SKIPPED (not silently): " + msg)
	}
	return path
}

// headAgentBinary returns this build's cercano: CERCANO_TEST_AGENT_BIN when
// set (a prebuilt binary, for running the compiled test where Go is not
// installed), else a fresh build.
func headAgentBinary(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("CERCANO_TEST_AGENT_BIN"); path != "" {
		if testing.Short() {
			t.Skip("runs the cercano binary")
		}
		return path
	}
	return buildAgentBinary(t)
}

// setCoordinatorFlag puts the flag in state for this process — the one
// running upgradeRestartCommand — and checks that the code reads it so.
func setCoordinatorFlag(t *testing.T, f flagState) {
	t.Helper()
	t.Setenv(paths.PerProfileAgentsEnv, f.value)
	if !f.set {
		os.Unsetenv(paths.PerProfileAgentsEnv)
	}
	if got, want := paths.PerProfileAgents(), f.set && f.value == "1"; got != want {
		t.Fatalf("PerProfileAgents() = %v with the flag %s, want %v", got, f, want)
	}
}

// transitionHome makes a short private home for the test and points this
// process (the coordinator) at it.
func transitionHome(t *testing.T) string {
	t.Helper()
	home, err := os.MkdirTemp("/tmp", "cpft-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	if err := os.MkdirAll(filepath.Join(home, "t"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

// deadOllama is a loopback URL nothing listens on, so no agent reaches a
// real Ollama.
func deadOllama(t *testing.T) string {
	return "http://127.0.0.1:" + freeLoopbackPort(t)
}

func freeLoopbackPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

func seedProfileConfig(t *testing.T, home, profile, port, ollama string) {
	t.Helper()
	dir := filepath.Join(paths.ProfilesRootIn(home), profile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf("locus_mode: open_only\nopen_runtime: ollama\nollama_url: %s\n", ollama)
	if port != "" {
		body += fmt.Sprintf("port: %q\n", port)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// transitionEnv is an agent's environment: agentEnv's isolation, the flag in
// state f, and CERCANO_PORT as given (empty: a named profile's ephemeral port).
func transitionEnv(home, profile string, f flagState, port, ollama string) []string {
	var env []string
	for _, kv := range agentEnv(home, profile) {
		if strings.HasPrefix(kv, paths.PerProfileAgentsEnv+"=") || strings.HasPrefix(kv, "TMPDIR=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "TMPDIR="+filepath.Join(home, "t"), "CERCANO_PORT="+port, "OLLAMA_URL="+ollama,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"))
	if f.set {
		env = append(env, paths.PerProfileAgentsEnv+"="+f.value)
	}
	return env
}

// child is a process this test started. Only children are ever signalled.
type child struct {
	cmd  *exec.Cmd
	out  *lockedWriter
	done chan struct{}
	err  error
}

func (c *child) exited() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *child) output() string { return c.out.String() }

func startChild(t *testing.T, bin string, args, env []string, dir string) *child {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Dir = dir
	c := &child{cmd: cmd, out: &lockedWriter{w: &bytes.Buffer{}}, done: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = c.out, c.out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", bin, err)
	}
	go func() { c.err = cmd.Wait(); close(c.done) }()
	t.Cleanup(func() { c.stop(t) })
	return c
}

// stop interrupts the child (its process group, so its workers too) and
// kills it if it does not exit.
func (c *child) stop(t *testing.T) {
	if c.exited() {
		return
	}
	_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGINT)
	select {
	case <-c.done:
	case <-time.After(20 * time.Second):
		_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
		<-c.done
		t.Errorf("pid %d did not exit within 20s of an interrupt", c.cmd.Process.Pid)
	}
}

// startTransitionAgent runs bin's `cercano agent` for profile and waits for
// its registry record.
func startTransitionAgent(t *testing.T, bin, home, profile string, f flagState, port, ollama string) (*child, hostregistry.Record) {
	t.Helper()
	c := startChild(t, bin, []string{"agent"}, transitionEnv(home, profile, f, port, ollama), home)
	rec := waitRegistered(t, home, profile, c)
	return c, rec
}

func waitRegistered(t *testing.T, home, profile string, c *child) hostregistry.Record {
	t.Helper()
	dir := filepath.Join(paths.ProfilesRootIn(home), profile)
	deadline := time.Now().Add(60 * time.Second)
	for {
		rec, err := hostregistry.Read(dir)
		if err == nil && rec.PID == c.cmd.Process.Pid {
			if conn, err := net.DialTimeout("tcp", rec.Addr, time.Second); err == nil {
				conn.Close()
				return rec
			}
		}
		if c.exited() {
			t.Fatalf("agent for %q exited before registering: %v\n%s", profile, c.err, c.output())
		}
		if time.Now().After(deadline) {
			t.Fatalf("agent for %q did not register within 60s\n%s", profile, c.output())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// servingAgent reports the agent now registered for profile and answering
// GetHostIdentity as that profile and instance, if any.
func servingAgent(home, profile string) (hostregistry.Record, bool) {
	rec, err := hostregistry.Read(filepath.Join(paths.ProfilesRootIn(home), profile))
	if err != nil {
		return hostregistry.Record{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	id, err := recordedHostIdentity(ctx, rec)
	if err != nil || id.GetProfile() != profile || id.GetInstanceId() != rec.InstanceID {
		return hostregistry.Record{}, false
	}
	return rec, true
}

func procEnviron(pid int) (map[string]string, error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, kv := range strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00") {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	return env, nil
}

// listenAddrs lists the local addresses of LISTEN sockets on port.
func listenAddrs(t *testing.T, port uint16) []netip.Addr {
	t.Helper()
	var addrs []netip.Addr
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(table)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			f := strings.Fields(line)
			if len(f) < 4 || f[3] != "0A" {
				continue
			}
			hostHex, portHex, _ := strings.Cut(f[1], ":")
			p, err := strconv.ParseUint(portHex, 16, 16)
			if err != nil || uint16(p) != port {
				continue
			}
			if addr, ok := parseProcAddrForTest(hostHex); ok {
				addrs = append(addrs, addr.Unmap())
			}
		}
	}
	return addrs
}

// parseProcAddrForTest decodes a /proc/net/tcp address: network-order bytes
// printed as native-endian (little-endian here) 32-bit words.
func parseProcAddrForTest(s string) (netip.Addr, bool) {
	if len(s) != 8 && len(s) != 32 {
		return netip.Addr{}, false
	}
	var b []byte
	for i := 0; i < len(s); i += 8 {
		w, err := strconv.ParseUint(s[i:i+8], 16, 32)
		if err != nil {
			return netip.Addr{}, false
		}
		b = append(b, byte(w), byte(w>>8), byte(w>>16), byte(w>>24))
	}
	return netip.AddrFromSlice(b)
}

// linuxReplay stands in for brewrestart's Darwin coordinator; see the file
// comment. It records what it did for the assertions.
type linuxReplay struct {
	t            *testing.T
	owned        map[int]bool // PIDs of processes this test started
	mu           sync.Mutex
	checked      int // launch environments the replacement check was applied to
	stopped      []int
	replacements []*child
}

func newLinuxReplay(t *testing.T, owned ...*child) *linuxReplay {
	r := &linuxReplay{t: t, owned: map[int]bool{}}
	for _, c := range owned {
		r.owned[c.cmd.Process.Pid] = true
	}
	return r
}

func (r *linuxReplay) deps(executable string) upgradeRestartDeps {
	return upgradeRestartDeps{
		legacyPort:    upgradeRestartPort,
		executable:    func() (string, error) { return executable, nil },
		profileAgents: runningProfileAgents,
		restartProfile: func(ctx context.Context, exe string, a profileAgent, endpoint netip.AddrPort, check brewrestart.ReplacementCheck) (bool, error) {
			probe := profileAgentRegistered(a.Profile, a.Dir)
			return r.restart(ctx, exe, endpoint, check, func(ctx context.Context, c *child) bool { return probe(ctx, c.cmd.Process.Pid) })
		},
		restartAddress: func(ctx context.Context, exe string, endpoint netip.AddrPort, check brewrestart.ReplacementCheck) (bool, error) {
			return r.restart(ctx, exe, endpoint, check, func(ctx context.Context, c *child) bool {
				id, found, err := brewrestart.ListenerProcess(endpoint)
				return err == nil && found && id.PID == c.cmd.Process.Pid
			})
		},
		// The shared model host is not part of these transitions.
		relaunchModelHost: func(context.Context, string) (bool, error) { return false, nil },
	}
}

func (r *linuxReplay) restart(ctx context.Context, exe string, endpoint netip.AddrPort, check brewrestart.ReplacementCheck, ready func(context.Context, *child) bool) (bool, error) {
	id, found, err := brewrestart.ListenerProcess(endpoint)
	if err != nil || !found {
		return false, err
	}
	if !r.owned[id.PID] {
		r.t.Fatalf("linuxReplay: %s is held by pid %d, which this test did not start; refusing to act on it", endpoint, id.PID)
	}
	// Capture the launch state before the destructive boundary.
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", id.PID))
	if err != nil {
		return false, err
	}
	environ, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", id.PID))
	if err != nil {
		return false, err
	}
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", id.PID))
	if err != nil {
		return false, err
	}
	args := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
	env := strings.Split(strings.TrimRight(string(environ), "\x00"), "\x00")
	r.mu.Lock()
	r.checked++
	r.mu.Unlock()
	if err := brewrestart.CheckReplacement(check, env); err != nil {
		return false, err
	}

	c, err := agentclient.DialExisting(ctx, endpoint.String())
	if err != nil {
		return false, fmt.Errorf("shutdown request failed; no replacement started: %w", err)
	}
	err = c.ShutdownAgent(ctx, "Homebrew installation updated (Linux emulation)")
	c.Close()
	if err != nil {
		return false, fmt.Errorf("shutdown request failed; no replacement started: %w", err)
	}
	for {
		gone, err := brewrestart.ProcessExited(id)
		if err != nil {
			return false, fmt.Errorf("old agent has not confirmed exit; no replacement started: %w", err)
		}
		if gone {
			break
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("old agent has not confirmed exit; no replacement started: %w", ctx.Err())
		case <-time.After(25 * time.Millisecond):
		}
	}
	r.mu.Lock()
	r.stopped = append(r.stopped, id.PID)
	r.mu.Unlock()

	repl := startChild(r.t, exe, args[1:], env, cwd)
	r.mu.Lock()
	r.replacements = append(r.replacements, repl)
	r.owned[repl.cmd.Process.Pid] = true
	r.mu.Unlock()
	for {
		if repl.exited() {
			return false, fmt.Errorf("replacement started but readiness was not confirmed; check agent logs before retrying: replacement exited during startup: %v", repl.err)
		}
		if ready(ctx, repl) {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("replacement started but readiness was not confirmed; check agent logs before retrying: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func runUpgrade(t *testing.T, r *linuxReplay, exe string, args ...string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := upgradeRestartCommand(append([]string{"--timeout", "30s"}, args...), &out, r.deps(exe))
	t.Logf("restart-after-upgrade %v (coordinator flag %v): exit %d\n%s", args, paths.PerProfileAgents(), code, out.String())
	return code, out.String()
}

// coordinatorFlags are the coordinator flag states every transition runs
// under: the decision must not depend on them.
var coordinatorFlags = []flagState{flagAbsent, flagOn}

// assertRestarted checks that a new host, launched from the old one's
// captured environment, now serves profile in place of old.
func assertRestarted(t *testing.T, r *linuxReplay, home, profile string, old *child, rec hostregistry.Record, wantFlag string) hostregistry.Record {
	t.Helper()
	if !old.exited() || len(r.stopped) != 1 || len(r.replacements) != 1 {
		t.Fatalf("the old host should be replaced once: exited=%v stopped=%v replacements=%d", old.exited(), r.stopped, len(r.replacements))
	}
	now, ok := servingAgent(home, profile)
	if !ok || now.PID != r.replacements[0].cmd.Process.Pid || now.InstanceID == rec.InstanceID {
		t.Fatalf("profile %s should be served by the replacement (pid %d): %+v (old %+v)", profile, r.replacements[0].cmd.Process.Pid, now, rec)
	}
	env, err := procEnviron(now.PID)
	if err != nil || env[paths.PerProfileAgentsEnv] != wantFlag {
		t.Errorf("replacement environment flag = %q (%v), want the captured %q", env[paths.PerProfileAgentsEnv], err, wantFlag)
	}
	return now
}

// assertLeftServing checks that old was judged, never stopped, and still
// serves profile as the same process and instance.
func assertLeftServing(t *testing.T, r *linuxReplay, home, profile string, old *child, rec hostregistry.Record) {
	t.Helper()
	if r.checked != 1 {
		t.Fatalf("the replacement check should have judged the host once, ran %d times", r.checked)
	}
	if len(r.stopped) != 0 || len(r.replacements) != 0 || old.exited() {
		t.Fatalf("the host must not be stopped: stopped=%v replacements=%d exited=%v", r.stopped, len(r.replacements), old.exited())
	}
	now, ok := servingAgent(home, profile)
	if !ok || now.PID != rec.PID || now.InstanceID != rec.InstanceID {
		t.Fatalf("profile %s should still be served by the same host: %+v (was %+v)", profile, now, rec)
	}
}

// A flag-on named host is restarted from its captured launch state, flag
// included, whatever the coordinator's own flag — with the flag off in the
// coordinator too, since the replacement inherits the host's environment.
func TestFlagTransitionRestartsFlagOnNamedHost(t *testing.T) {
	bin := headAgentBinary(t)
	for _, coordinator := range coordinatorFlags {
		t.Run("coordinator "+coordinator.String(), func(t *testing.T) {
			home := transitionHome(t)
			ollama := deadOllama(t)
			seedProfileConfig(t, home, "work", "", ollama)
			old, rec := startTransitionAgent(t, bin, home, "work", flagOn, "", ollama)
			setCoordinatorFlag(t, coordinator)

			r := newLinuxReplay(t, old)
			code, out := runUpgrade(t, r, bin)
			if code != 0 || !strings.Contains(out, `Restarted agent for profile "work".`) || strings.Contains(out, "Left the agent") {
				t.Fatalf("exit %d, output:\n%s", code, out)
			}
			assertRestarted(t, r, home, "work", old, rec, "1")
		})
	}
}

// A named host started by f79bf87a, which predates the flag, has no flag in
// its environment, so its replacement would refuse the profile. Whatever the
// coordinator's flag, the host is judged from its captured environment before
// anything is stopped, reported, and keeps serving; the command succeeds.
func TestFlagTransitionLeavesPreFlagNamedHostServing(t *testing.T) {
	f79 := historicalAgentBinary(t, "f79bf87a")
	bin := headAgentBinary(t)
	for _, coordinator := range coordinatorFlags {
		t.Run("coordinator "+coordinator.String(), func(t *testing.T) {
			home := transitionHome(t)
			ollama := deadOllama(t)
			seedProfileConfig(t, home, "work", "", ollama)
			old, rec := startTransitionAgent(t, f79, home, "work", flagAbsent, "", ollama)
			setCoordinatorFlag(t, coordinator)

			r := newLinuxReplay(t, old)
			code, out := runUpgrade(t, r, bin)
			want := `Left the agent for profile "work" running: it was started without ` + paths.PerProfileAgentsEnv + `=1`
			if code != 0 || !strings.Contains(out, want) || strings.Contains(out, "failed") {
				t.Fatalf("exit %d, want 0 and %q:\n%s", code, want, out)
			}
			assertLeftServing(t, r, home, "work", old, rec)
		})
	}
}

// --address naming a named profile's host gets the same decision as
// discovery: a flag-on host is replaced and confirmed through the profile's
// record (so an ephemeral-port host may come back on a new port), and a
// pre-flag f79bf87a host is left serving — on an ephemeral or a fixed port,
// whatever the coordinator's flag.
func TestFlagTransitionExplicitAddressOfNamedHost(t *testing.T) {
	bin := headAgentBinary(t)
	for _, tc := range []struct {
		name      string
		historic  bool
		fixedPort bool
	}{
		{"flag-on host, ephemeral port", false, false},
		{"flag-on host, fixed port", false, true},
		{"f79bf87a host, ephemeral port", true, false},
		{"f79bf87a host, fixed port", true, true},
	} {
		for _, coordinator := range coordinatorFlags {
			t.Run(tc.name+", coordinator "+coordinator.String(), func(t *testing.T) {
				agentBin, hostFlag := bin, flagOn
				if tc.historic {
					agentBin, hostFlag = historicalAgentBinary(t, "f79bf87a"), flagAbsent
				}
				home := transitionHome(t)
				ollama := deadOllama(t)
				port := ""
				if tc.fixedPort {
					port = freeLoopbackPort(t)
				}
				seedProfileConfig(t, home, "work", "", ollama)
				old, rec := startTransitionAgent(t, agentBin, home, "work", hostFlag, port, ollama)
				if tc.fixedPort && rec.Addr != "127.0.0.1:"+port {
					t.Fatalf("host with CERCANO_PORT=%s registered %s", port, rec.Addr)
				}
				setCoordinatorFlag(t, coordinator)

				r := newLinuxReplay(t, old)
				code, out := runUpgrade(t, r, bin, "--address", rec.Addr)
				if tc.historic {
					if code != 0 || !strings.Contains(out, `Left the agent for profile "work" running`) || strings.Contains(out, "failed") {
						t.Fatalf("exit %d:\n%s", code, out)
					}
					assertLeftServing(t, r, home, "work", old, rec)
					return
				}
				if code != 0 || !strings.Contains(out, `Restarted agent for profile "work".`) {
					t.Fatalf("exit %d:\n%s", code, out)
				}
				now := assertRestarted(t, r, home, "work", old, rec, "1")
				if tc.fixedPort && now.Addr != rec.Addr {
					t.Errorf("a fixed-port host should come back at %s, registered %s", rec.Addr, now.Addr)
				}
			})
		}
	}
}

// Upgrade of the default profile from f79bf87a (loopback host) to this
// build: the replacement inherits the host's flagless environment, so it
// restores the pre-profile all-interfaces bind on the same port and
// registers a loopback address, whatever the coordinator's flag.
func TestFlagTransitionDefaultHostUpgradeFromF79(t *testing.T) {
	f79 := historicalAgentBinary(t, "f79bf87a")
	bin := headAgentBinary(t)
	for _, coordinator := range coordinatorFlags {
		t.Run("coordinator "+coordinator.String(), func(t *testing.T) {
			home := transitionHome(t)
			ollama := deadOllama(t)
			port := freeLoopbackPort(t)
			seedProfileConfig(t, home, "default", port, ollama)
			old, rec := startTransitionAgent(t, f79, home, "default", flagAbsent, port, ollama)
			p, _ := strconv.ParseUint(port, 10, 16)
			if got := listenAddrs(t, uint16(p)); len(got) != 1 || !got[0].IsLoopback() {
				t.Fatalf("f79bf87a default host listens on %v, want loopback only", got)
			}
			setCoordinatorFlag(t, coordinator)

			r := newLinuxReplay(t, old)
			code, out := runUpgrade(t, r, bin)
			if code != 0 || !strings.Contains(out, `Restarted agent for profile "default".`) {
				t.Fatalf("exit %d:\n%s", code, out)
			}
			now := assertRestarted(t, r, home, "default", old, rec, "")
			if now.Addr != "127.0.0.1:"+port {
				t.Errorf("replacement registered %s, want 127.0.0.1:%s", now.Addr, port)
			}
			if got := listenAddrs(t, uint16(p)); len(got) != 1 || !got[0].IsUnspecified() {
				t.Errorf("replacement listens on %v, want every interface (flag off)", got)
			}
		})
	}
}
