package main

import (
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
)

// embeddedGRPCBindAddr is where MCP embedded mode serves its private agent:
// loopback only, on a free port.
const embeddedGRPCBindAddr = "127.0.0.1:0"

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

// agentRunningError reports that another agent holds the profile's lock.
type agentRunningError struct {
	profile string
	pid     int // 0 when the running agent's record is missing or unreadable
}

func (e *agentRunningError) Error() string {
	if e.pid > 0 {
		return fmt.Sprintf("profile %q already has a running agent (pid %d)", e.profile, e.pid)
	}
	return fmt.Sprintf("profile %q already has a running agent (its %s is missing or unreadable)", e.profile, hostregistry.FileName)
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

// acquireAgentHost takes the profile's agent lock. When another agent holds
// it, the error names that agent's PID from its registry record. Holding the
// lock means any record already present belongs to an agent that is gone, so
// it is removed.
func acquireAgentHost(profile string) (*agentHost, error) {
	if err := paths.ValidateProfileName(profile); err != nil {
		return nil, err
	}
	dir := paths.ProfileDir(profile)
	release, ok, err := filelock.TryLock(filepath.Join(dir, agentLockName))
	if err != nil {
		return nil, fmt.Errorf("take the agent lock for profile %q: %w", profile, err)
	}
	if !ok {
		running := &agentRunningError{profile: profile}
		if rec, err := hostregistry.Read(dir); err == nil && rec.PID > 0 {
			running.pid = rec.PID
		}
		return nil, running
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

// embeddedHostIdentity identifies MCP embedded mode's private agent. That
// agent is not registered and takes no profile lock.
func embeddedHostIdentity() server.HostIdentity {
	id, err := newInstanceID()
	if err != nil {
		id = ""
	}
	return server.HostIdentity{Profile: activeProfile(), PID: os.Getpid(), InstanceID: id, Version: version}
}
