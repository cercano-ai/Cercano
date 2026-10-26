package agentclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"cercano/source/server/internal/hostregistry"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/proto"
)

// identityServer is an in-process agent host that answers GetHostIdentity.
type identityServer struct {
	proto.UnimplementedAgentServer
	profile  string
	instance string
}

func (s *identityServer) GetHostIdentity(context.Context, *proto.GetHostIdentityRequest) (*proto.HostIdentity, error) {
	return &proto.HostIdentity{Profile: s.profile, Pid: int32(os.Getpid()), InstanceId: s.instance, Version: "test"}, nil
}

type testHost struct {
	addr     string
	profile  string
	instance string
	srv      *grpc.Server
}

func newTestInstanceID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// startTestHost serves an identityServer on a fresh loopback port. It is
// safe to call from any goroutine.
func startTestHost(t *testing.T, profile, instance string) (*testHost, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	srv := grpc.NewServer()
	proto.RegisterAgentServer(srv, &identityServer{profile: profile, instance: instance})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return &testHost{addr: lis.Addr().String(), profile: profile, instance: instance, srv: srv}, nil
}

func mustStartTestHost(t *testing.T, profile, instance string) *testHost {
	t.Helper()
	h, err := startTestHost(t, profile, instance)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// writeTestRecord stores a registry record in profile's directory.
func writeTestRecord(profile, recordProfile, addr, instance string) error {
	return hostregistry.Write(paths.ProfileDir(profile), hostregistry.Record{
		Profile:    recordProfile,
		PID:        os.Getpid(),
		Addr:       addr,
		InstanceID: instance,
		Version:    "test",
		StartedAt:  time.Now().UTC(),
	})
}

func isolateProfileHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CERCANO_PROFILE", "")
}

func useLaunchFunc(t *testing.T, fn func(profile, logPath string) (<-chan struct{}, error)) {
	t.Helper()
	old := launchFunc
	launchFunc = fn
	t.Cleanup(func() { launchFunc = old })
}

// fakeLauncher stands in for spawning `cercano agent`: it starts an
// in-process host for the profile and registers it the way an agent does.
type fakeLauncher struct {
	t        *testing.T
	delay    time.Duration
	launches atomic.Int32
	mu       sync.Mutex
	hosts    []*testHost
}

func (f *fakeLauncher) launch(profile, logPath string) (<-chan struct{}, error) {
	f.launches.Add(1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	h, err := startTestHost(f.t, profile, newTestInstanceID())
	if err != nil {
		return nil, err
	}
	if err := writeTestRecord(profile, profile, h.addr, h.instance); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.hosts = append(f.hosts, h)
	f.mu.Unlock()
	return nil, nil
}

func (f *fakeLauncher) last() *testHost {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.hosts) == 0 {
		return nil
	}
	return f.hosts[len(f.hosts)-1]
}

// identityOf asks the host the client is currently connected to who it is.
func identityOf(t *testing.T, c *Client) *proto.HostIdentity {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	id, err := proto.NewAgentClient(c.readConn()).GetHostIdentity(ctx, &proto.GetHostIdentityRequest{})
	if err != nil {
		t.Fatalf("GetHostIdentity through the client: %v", err)
	}
	return id
}

func TestDialProfileLaunchesAndVerifiesIdentity(t *testing.T) {
	isolateProfileHome(t)
	f := &fakeLauncher{t: t}
	useLaunchFunc(t, f.launch)

	c, err := DialProfile(context.Background(), "work")
	if err != nil {
		t.Fatalf("DialProfile: %v", err)
	}
	defer c.Close()

	if n := f.launches.Load(); n != 1 {
		t.Fatalf("launches = %d, want 1", n)
	}
	if c.Profile() != "work" {
		t.Errorf("Profile() = %q, want work", c.Profile())
	}
	id := identityOf(t, c)
	if id.GetProfile() != "work" || id.GetInstanceId() != f.last().instance {
		t.Errorf("connected to %s/%s, want work/%s", id.GetProfile(), id.GetInstanceId(), f.last().instance)
	}
	if !c.AutoLaunched {
		t.Error("AutoLaunched = false after a launch")
	}
	wantLog := filepath.Join(paths.ProfileDir("work"), "logs", "agent.log")
	if c.ServerLog != wantLog {
		t.Errorf("ServerLog = %q, want %q", c.ServerLog, wantLog)
	}
}

func TestDialProfileReusesRunningHost(t *testing.T) {
	isolateProfileHome(t)
	h := mustStartTestHost(t, "work", newTestInstanceID())
	if err := writeTestRecord("work", "work", h.addr, h.instance); err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	useLaunchFunc(t, func(string, string) (<-chan struct{}, error) {
		launches.Add(1)
		return nil, errors.New("launch not expected")
	})

	c, err := DialProfile(context.Background(), "work")
	if err != nil {
		t.Fatalf("DialProfile: %v", err)
	}
	defer c.Close()
	if n := launches.Load(); n != 0 {
		t.Fatalf("launches = %d, want 0", n)
	}
	if c.AutoLaunched {
		t.Error("AutoLaunched = true without a launch")
	}
	if id := identityOf(t, c); id.GetInstanceId() != h.instance {
		t.Errorf("connected to instance %q, want %q", id.GetInstanceId(), h.instance)
	}
}

func TestDialProfileRejectsWrongIdentity(t *testing.T) {
	cases := []struct {
		name string
		// what the live host at the recorded address reports
		hostProfile, hostInstance string
		// what the record in the work profile says
		recordInstance string
	}{
		// A stale record whose port now belongs to another profile's agent.
		{"another profile's agent on the recorded port", "other", "other-1", "work-stale"},
		// Profile alone must disqualify the host, even if the instance matches.
		{"another profile reporting the recorded instance", "other", "same-id", "same-id"},
		// Instance alone must disqualify the host, even if the profile matches.
		{"this profile with a different instance", "work", "work-new", "work-old"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateProfileHome(t)
			wrong := mustStartTestHost(t, tc.hostProfile, tc.hostInstance)
			if err := writeTestRecord("work", "work", wrong.addr, tc.recordInstance); err != nil {
				t.Fatal(err)
			}
			f := &fakeLauncher{t: t}
			useLaunchFunc(t, f.launch)

			c, err := DialProfile(context.Background(), "work")
			if err != nil {
				t.Fatalf("DialProfile: %v", err)
			}
			defer c.Close()
			if n := f.launches.Load(); n != 1 {
				t.Fatalf("launches = %d, want 1", n)
			}
			id := identityOf(t, c)
			if id.GetProfile() != "work" || id.GetInstanceId() != f.last().instance {
				t.Fatalf("connected to %s/%s, want work/%s", id.GetProfile(), id.GetInstanceId(), f.last().instance)
			}
		})
	}
}

func TestConcurrentDialProfileLaunchesOnce(t *testing.T) {
	isolateProfileHome(t)
	f := &fakeLauncher{t: t, delay: 200 * time.Millisecond}
	useLaunchFunc(t, f.launch)

	const clients = 6
	var wg sync.WaitGroup
	results := make([]*Client, clients)
	errs := make([]error, clients)
	for i := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = DialProfile(context.Background(), "work")
		}()
	}
	wg.Wait()
	for i := range clients {
		if errs[i] != nil {
			t.Fatalf("client %d: %v", i, errs[i])
		}
		defer results[i].Close()
	}
	if n := f.launches.Load(); n != 1 {
		t.Fatalf("launches = %d, want exactly 1", n)
	}
	want := f.last().instance
	for i, c := range results {
		if id := identityOf(t, c); id.GetInstanceId() != want {
			t.Errorf("client %d on instance %q, want %q", i, id.GetInstanceId(), want)
		}
	}
}

func TestReconnectFollowsNewPort(t *testing.T) {
	isolateProfileHome(t)
	f := &fakeLauncher{t: t}
	useLaunchFunc(t, f.launch)

	c, err := DialProfile(context.Background(), "work")
	if err != nil {
		t.Fatalf("DialProfile: %v", err)
	}
	defer c.Close()
	first := f.last()

	// Restart the host on a new port with a new record.
	second := mustStartTestHost(t, "work", newTestInstanceID())
	first.srv.Stop()
	if err := writeTestRecord("work", "work", second.addr, second.instance); err != nil {
		t.Fatal(err)
	}

	// The fast burst's first attempt runs after 1s; allow its whole burst.
	deadline := time.Now().Add(1*time.Second + 2*time.Second + 4*time.Second + 3*time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		id, err := proto.NewAgentClient(c.readConn()).GetHostIdentity(ctx, &proto.GetHostIdentityRequest{})
		cancel()
		if err == nil && id.GetInstanceId() == second.instance {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("client did not move to the restarted host (last err %v, state %s)", err, c.State())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got := c.currentAddr(); got != second.addr {
		t.Errorf("client address = %q, want %q", got, second.addr)
	}
	if n := f.launches.Load(); n != 1 {
		t.Errorf("launches = %d, want 1 (reconnect should follow the record, not launch)", n)
	}
}

func TestDialProfileReportsEarlyExitLogTail(t *testing.T) {
	isolateProfileHome(t)
	logPath := filepath.Join(paths.ProfileDir("work"), "logs", "agent.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("an older run's output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const exitLine = `Error: the previous agent for profile "work" (pid 99) is still finishing its work; try again shortly`
	useLaunchFunc(t, func(profile, gotLog string) (<-chan struct{}, error) {
		if gotLog != logPath {
			return nil, fmt.Errorf("log path %q, want %q", gotLog, logPath)
		}
		lf, err := os.OpenFile(gotLog, os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, err
		}
		_, _ = lf.WriteString("starting agent\n" + exitLine + "\n")
		_ = lf.Close()
		exited := make(chan struct{})
		close(exited)
		return exited, nil
	})

	start := time.Now()
	c, err := DialProfile(context.Background(), "work")
	if err == nil {
		c.Close()
		t.Fatal("DialProfile succeeded with an agent that exited")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Errorf("early exit took %s to report; want it noticed without the full wait", elapsed)
	}
	msg := err.Error()
	if !strings.Contains(msg, exitLine) {
		t.Errorf("error does not carry the agent's last log line:\n%s", msg)
	}
	if strings.Contains(msg, "older run") {
		t.Errorf("error includes output from before this launch:\n%s", msg)
	}
}

func TestDialProfileRejectsInvalidName(t *testing.T) {
	isolateProfileHome(t)
	var launches atomic.Int32
	useLaunchFunc(t, func(string, string) (<-chan struct{}, error) {
		launches.Add(1)
		return nil, errors.New("launch not expected")
	})
	if c, err := DialProfile(context.Background(), "../x"); err == nil {
		c.Close()
		t.Fatal("DialProfile accepted an invalid profile name")
	}
	if launches.Load() != 0 {
		t.Fatal("launched for an invalid profile name")
	}
	if _, err := os.Stat(paths.ProfilesRoot()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("profiles directory created for an invalid name: %v", err)
	}
}

func TestDialClientHasNoProfile(t *testing.T) {
	if (&Client{}).Profile() != "" {
		t.Fatal("a client dialed by address reports a profile")
	}
}

// shortenLaunchWaits sets the base launch wait and its hard cap for a test.
func shortenLaunchWaits(t *testing.T, base, limit time.Duration) {
	t.Helper()
	oldBase, oldMax := profileLaunchWait, profileLaunchMaxWait
	profileLaunchWait, profileLaunchMaxWait = base, limit
	t.Cleanup(func() { profileLaunchWait, profileLaunchMaxWait = oldBase, oldMax })
}

func TestDialProfileWaitsForLiveChildPastBaseWait(t *testing.T) {
	// A new agent behind a draining predecessor stays alive while it waits
	// for the profile lock, which can outlast the base wait.
	isolateProfileHome(t)
	shortenLaunchWaits(t, 300*time.Millisecond, 5*time.Second)
	alive := make(chan struct{})
	t.Cleanup(func() { close(alive) })
	var launches atomic.Int32
	var instance atomic.Value
	useLaunchFunc(t, func(profile, logPath string) (<-chan struct{}, error) {
		launches.Add(1)
		go func() {
			time.Sleep(900 * time.Millisecond)
			h, err := startTestHost(t, profile, newTestInstanceID())
			if err != nil {
				t.Errorf("start host: %v", err)
				return
			}
			instance.Store(h.instance)
			if err := writeTestRecord(profile, profile, h.addr, h.instance); err != nil {
				t.Errorf("write record: %v", err)
			}
		}()
		return alive, nil
	})

	c, err := DialProfile(context.Background(), "work")
	if err != nil {
		t.Fatalf("DialProfile gave up on a live agent: %v", err)
	}
	defer c.Close()
	if n := launches.Load(); n != 1 {
		t.Fatalf("launches = %d, want 1", n)
	}
	if id := identityOf(t, c); id.GetInstanceId() != instance.Load() {
		t.Fatalf("connected to instance %q, want %v", id.GetInstanceId(), instance.Load())
	}
}

func TestDialProfileGivesUpOnLiveChildAtCap(t *testing.T) {
	isolateProfileHome(t)
	shortenLaunchWaits(t, 200*time.Millisecond, 700*time.Millisecond)
	alive := make(chan struct{})
	t.Cleanup(func() { close(alive) })
	useLaunchFunc(t, func(string, string) (<-chan struct{}, error) { return alive, nil })

	start := time.Now()
	c, err := DialProfile(context.Background(), "work")
	elapsed := time.Since(start)
	if err == nil {
		c.Close()
		t.Fatal("DialProfile succeeded with no host")
	}
	if elapsed < 700*time.Millisecond {
		t.Errorf("gave up after %s, before the %s cap, while the agent was alive", elapsed, 700*time.Millisecond)
	}
	if elapsed > 3*time.Second {
		t.Errorf("gave up after %s, well past the %s cap", elapsed, 700*time.Millisecond)
	}
	if !strings.Contains(err.Error(), "did not answer within") {
		t.Errorf("error is not a timeout: %v", err)
	}
}

func TestDialProfileRejectsRecordNamingAnotherProfile(t *testing.T) {
	// The host answers as this profile with the recorded instance, but the
	// record itself claims another profile: it cannot be this profile's
	// agent's record.
	isolateProfileHome(t)
	h := mustStartTestHost(t, "work", "same-id")
	if err := writeTestRecord("work", "other", h.addr, "same-id"); err != nil {
		t.Fatal(err)
	}
	f := &fakeLauncher{t: t}
	useLaunchFunc(t, f.launch)

	c, err := DialProfile(context.Background(), "work")
	if err != nil {
		t.Fatalf("DialProfile: %v", err)
	}
	defer c.Close()
	if n := f.launches.Load(); n != 1 {
		t.Fatalf("launches = %d, want 1", n)
	}
	if id := identityOf(t, c); id.GetInstanceId() != f.last().instance {
		t.Fatalf("connected to instance %q, want %q", id.GetInstanceId(), f.last().instance)
	}
}
