package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cercano/source/server/internal/filelock"
	"cercano/source/server/internal/hostregistry"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// agentHandoverWait is how long a new agent waits for the profile lock while
// the agent holding it is starting or draining; tests shorten it.
var agentHandoverWait = 15 * time.Second

// agentHandoverPoll is how often a waiting agent checks whether the lock
// holder has become a healthy agent.
const agentHandoverPoll = 250 * time.Millisecond

// agentProbeTimeout bounds one GetHostIdentity check of the lock holder.
const agentProbeTimeout = time.Second

// agentLockName is the per-profile lock an agent host holds while it runs.
const agentLockName = "agent.lock"

// agentBindAddr is the loopback address an agent host listens on. The default
// profile keeps the configured port so existing clients find it, as does any
// profile when CERCANO_PORT names a port; other profiles take a free port and
// publish it in their registry record.
func agentBindAddr(profile string, cfg config.Config) string {
	port := "0"
	if profile == paths.DefaultProfile || os.Getenv("CERCANO_PORT") != "" {
		port = cfg.Port
	}
	return net.JoinHostPort("127.0.0.1", port)
}

// listenFailure explains a failed listen, naming the port.
func listenFailure(bindAddr string, err error) error {
	_, port, splitErr := net.SplitHostPort(bindAddr)
	if splitErr != nil || port == "" || port == "0" {
		return fmt.Errorf("cannot listen on %s: %w", bindAddr, err)
	}
	return fmt.Errorf("cannot listen on port %s (%s): %w; another process may be using the port, stop it or set CERCANO_PORT to a free port", port, bindAddr, err)
}

// agentRunningError reports that a healthy agent already serves the profile.
type agentRunningError struct {
	profile string
	pid     int
}

func (e *agentRunningError) Error() string {
	return fmt.Sprintf("profile %q already has a running agent (pid %d)", e.profile, e.pid)
}

// agentFinishingError reports that the profile's lock stayed held for the
// whole handover wait without a healthy agent behind it: the previous agent
// is still draining in-flight turns (or still starting).
type agentFinishingError struct {
	profile string
	pid     int // 0 when the holder's record is missing or unreadable
}

func (e *agentFinishingError) Error() string {
	if e.pid > 0 {
		return fmt.Sprintf("the previous agent for profile %q (pid %d) is still finishing its work; try again shortly", e.profile, e.pid)
	}
	return fmt.Sprintf("the previous agent for profile %q is still finishing its work; try again shortly", e.profile)
}

// agentHost is this process's claim to serve a profile: the profile's agent
// lock, held for the process lifetime, and its registry record.
type agentHost struct {
	profile    string
	dir        string
	instanceID string
	started    time.Time

	mu         sync.Mutex
	registered bool
	release    func()
	closed     bool
}

// acquireAgentHost takes the profile's agent lock; see waitForAgentLock for
// what happens when another agent holds it. Holding the lock means any record
// already present belongs to an agent that is gone, so it is removed.
func acquireAgentHost(profile string) (*agentHost, error) {
	if err := paths.ValidateProfileName(profile); err != nil {
		return nil, err
	}
	dir := paths.ProfileDir(profile)
	release, err := waitForAgentLock(profile, dir)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(hostregistry.Path(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		release()
		return nil, fmt.Errorf("remove a stale agent record for profile %q: %w", profile, err)
	}
	id, err := newInstanceID()
	if err != nil {
		release()
		return nil, err
	}
	return &agentHost{profile: profile, dir: dir, instanceID: id, started: time.Now().UTC(), release: release}, nil
}

// waitForAgentLock takes the profile's agent lock. When a healthy agent holds
// it (its record's address answers GetHostIdentity as that agent), it returns
// agentRunningError at once. Otherwise the holder is still starting or is
// draining: GracefulStop closes the listener as the drain begins, so a
// refused connection is the normal state of an exiting agent. It then waits
// up to agentHandoverWait for the lock, re-checking for a healthy holder
// every agentHandoverPoll, and returns agentFinishingError if the lock is
// still held when the wait ends.
func waitForAgentLock(profile, dir string) (func(), error) {
	lockPath := filepath.Join(dir, agentLockName)
	deadline := time.Now().Add(agentHandoverWait)
	announced := false
	for {
		release, ok, err := filelock.TryLock(lockPath)
		if err != nil {
			return nil, fmt.Errorf("take the agent lock for profile %q: %w", profile, err)
		}
		if ok {
			return release, nil
		}
		pid, healthy := lockHolder(profile, dir)
		if healthy {
			return nil, &agentRunningError{profile: profile, pid: pid}
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, &agentFinishingError{profile: profile, pid: pid}
		}
		if !announced {
			fmt.Fprintf(os.Stderr, "Waiting for the previous agent for profile %q to exit...\n", profile)
			announced = true
		}
		ctx, cancel := context.WithTimeout(context.Background(), min(agentHandoverPoll, remaining))
		release, err = filelock.Lock(ctx, lockPath)
		cancel()
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("take the agent lock for profile %q: %w", profile, err)
		}
	}
}

// lockHolder reads the profile's record and reports the recorded PID (0 when
// there is no readable record) and whether that agent answers GetHostIdentity
// as itself within agentProbeTimeout.
func lockHolder(profile, dir string) (pid int, healthy bool) {
	rec, err := hostregistry.Read(dir)
	if err != nil {
		return 0, false
	}
	return rec.PID, agentAnswers(profile, rec)
}

func agentAnswers(profile string, rec hostregistry.Record) bool {
	id, err := recordedHostIdentity(context.Background(), rec)
	return err == nil && id.GetProfile() == profile && id.GetInstanceId() == rec.InstanceID
}

// recordedHostIdentity asks the agent at rec's address for its identity,
// within agentProbeTimeout and ctx.
func recordedHostIdentity(ctx context.Context, rec hostregistry.Record) (*proto.HostIdentity, error) {
	if rec.Addr == "" {
		return nil, errors.New("record has no address")
	}
	conn, err := grpc.NewClient(rec.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(ctx, agentProbeTimeout)
	defer cancel()
	return proto.NewAgentClient(conn).GetHostIdentity(ctx, &proto.GetHostIdentityRequest{})
}

func newInstanceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate agent instance ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// identity is what GetHostIdentity reports for this host.
func (h *agentHost) identity() server.HostIdentity {
	return server.HostIdentity{Profile: h.profile, PID: os.Getpid(), InstanceID: h.instanceID, Version: version}
}

// register publishes the address this host actually listens on.
func (h *agentHost) register(addr string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return errors.New("agent host already closed")
	}
	rec := hostregistry.Record{
		Profile:    h.profile,
		PID:        os.Getpid(),
		Addr:       addr,
		InstanceID: h.instanceID,
		Version:    version,
		StartedAt:  h.started,
	}
	if err := hostregistry.Write(h.dir, rec); err != nil {
		return fmt.Errorf("register agent for profile %q: %w", h.profile, err)
	}
	h.registered = true
	return nil
}

// close removes this host's record, if it still names this instance, and then
// releases the profile lock. It is safe to call more than once and from any
// goroutine.
func (h *agentHost) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	if h.registered {
		if err := hostregistry.Remove(h.dir, h.instanceID); err != nil {
			fmt.Fprintf(os.Stderr, "[WARN] %v\n", err)
		}
	}
	h.release()
}

// activeProfile is the validated CERCANO_PROFILE. Entry points reject an
// invalid name before any agent starts, so the fallback is never reached in
// practice.
func activeProfile() string {
	profile, err := paths.Profile()
	if err != nil {
		return paths.DefaultProfile
	}
	return profile
}
