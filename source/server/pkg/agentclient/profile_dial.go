package agentclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"cercano/source/server/internal/filelock"
	"cercano/source/server/internal/hostregistry"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/proto"
)

// profileLaunchWait is how long a launch waits for the new agent to register
// and answer GetHostIdentity. Tests shorten it.
var profileLaunchWait = 8 * time.Second

// profileLaunchMaxWait is the hard cap on that wait while the launched agent
// is still running. A new agent whose predecessor for the profile is still
// draining waits up to agentHandoverWait (15s, cmd/cercano/agent_host.go)
// for the profile's agent lock before it serves, so a live agent gets that
// long plus 2s to start listening. Keep it in step with agentHandoverWait.
// Tests shorten it.
var profileLaunchMaxWait = 17 * time.Second

// profileLaunchLockWait bounds the wait for another client that is launching
// the same profile's agent. That client holds the lock for at most about
// profileLaunchMaxWait, so this only trips if it is stuck.
const profileLaunchLockWait = 30 * time.Second

// hostIdentityTimeout bounds one GetHostIdentity check of a recorded host.
const hostIdentityTimeout = time.Second

// profileLaunchPoll is how often a launch re-checks the profile's record.
const profileLaunchPoll = 100 * time.Millisecond

// profileLaunchLockName is the per-profile lock clients hold while they
// check for and launch the profile's agent.
const profileLaunchLockName = "launch.lock"

// launchLogTailLines is how many of a failed agent's last log lines a launch
// error carries.
const launchLogTailLines = 10

// launchFunc starts the agent for profile, writing its output to logPath. The
// returned channel, when non-nil, is closed once the started process exits.
// Tests replace it to start an in-process host.
var launchFunc = launchProfileAgent

// ErrNoProfileAgent reports that the profile's record names no host that
// answers as this profile's registered agent.
var ErrNoProfileAgent = errors.New("no running agent for this profile")

// errNoProfileHost is ErrNoProfileAgent under its package-internal name.
var errNoProfileHost = ErrNoProfileAgent

// DialProfile connects to the agent host serving profile, launching one when
// none is running. It trusts the profile's agent.json only after the host at
// the recorded address answers GetHostIdentity with this profile and the
// record's instance ID, so a stale record whose port now belongs to another
// process or another profile's agent is never used.
//
// The launch is serialized per profile by <profile>/launch.lock: concurrent
// clients start one agent and all connect to it. Like Dial, the client
// recovers on its own when the host dies; each attempt resolves the profile's
// host again, because a restarted host may listen on a new port.
func DialProfile(ctx context.Context, profile string) (*Client, error) {
	if err := paths.ValidateProfileName(profile); err != nil {
		return nil, err
	}
	if err := paths.CheckHome(); err != nil {
		return nil, err
	}
	c, err := resolveProfileHost(ctx, profile)
	if err != nil {
		var logPath string
		c, logPath, err = launchProfileHost(ctx, profile)
		if err != nil {
			return nil, err
		}
		if logPath != "" {
			c.AutoLaunched = true
			c.ServerLog = logPath
		}
	}
	c.profile = profile
	c.stopWatch = make(chan struct{})
	go c.watchConn(context.Background())
	return c, nil
}

// DialProfileExisting connects to the profile's running agent, verified the
// same way as DialProfile, but never launches or respawns one. The error
// wraps ErrNoProfileAgent when no agent answers for the profile. The caller
// owns the connection's lifetime and retry policy; if the client reconnects,
// it only looks for the profile's running agent again.
func DialProfileExisting(ctx context.Context, profile string) (*Client, error) {
	if err := paths.ValidateProfileName(profile); err != nil {
		return nil, err
	}
	if err := paths.CheckHome(); err != nil {
		return nil, err
	}
	c, err := resolveProfileHost(ctx, profile)
	if err != nil {
		return nil, err
	}
	c.profile = profile
	c.noLaunch = true
	return c, nil
}

// Profile returns the profile a DialProfile or DialProfileExisting client
// serves; empty for a client dialed by address.
func (c *Client) Profile() string { return c.profile }

// resolveProfileHost connects to the host named by the profile's record if it
// answers as that profile's registered agent.
func resolveProfileHost(ctx context.Context, profile string) (*Client, error) {
	rec, err := hostregistry.Read(paths.ProfileDir(profile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errNoProfileHost
		}
		return nil, err
	}
	if rec.Profile != profile || rec.InstanceID == "" || rec.Addr == "" {
		return nil, errNoProfileHost
	}
	conn, err := grpc.NewClient(rec.Addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(maxGRPCMsgBytes),
			grpc.MaxCallRecvMsgSize(maxGRPCMsgBytes),
		),
	)
	if err != nil {
		return nil, err
	}
	agent := proto.NewAgentClient(conn)
	idCtx, cancel := context.WithTimeout(ctx, hostIdentityTimeout)
	id, err := agent.GetHostIdentity(idCtx, &proto.GetHostIdentityRequest{})
	cancel()
	if err != nil || id.GetProfile() != profile || id.GetInstanceId() != rec.InstanceID {
		_ = conn.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errNoProfileHost, err)
		}
		return nil, errNoProfileHost
	}
	return &Client{conn: conn, agent: agent, addr: rec.Addr}, nil
}

// launchProfileHost takes the profile's launch lock, checks again for a host
// another client may have started meanwhile, and otherwise launches the
// profile's agent and waits for it to answer. logPath is empty when no launch
// was needed.
func launchProfileHost(ctx context.Context, profile string) (c *Client, logPath string, err error) {
	dir := paths.ProfileDir(profile)
	lockCtx, cancel := context.WithTimeout(ctx, profileLaunchLockWait)
	release, err := filelock.Lock(lockCtx, filepath.Join(dir, profileLaunchLockName))
	cancel()
	if err != nil {
		return nil, "", fmt.Errorf("wait for another client starting the agent for profile %q: %w", profile, err)
	}
	defer release()

	if c, err := resolveProfileHost(ctx, profile); err == nil {
		return c, "", nil
	}

	logPath = filepath.Join(dir, "logs", "agent.log")
	offset := fileSize(logPath)
	exited, err := launchFunc(profile, logPath)
	if err != nil {
		return nil, "", fmt.Errorf("no agent for profile %q and could not auto-launch: %w", profile, err)
	}
	c, err = waitForProfileHost(ctx, profile, exited, logPath, offset)
	if err != nil {
		return nil, logPath, err
	}
	return c, logPath, nil
}

// waitForProfileHost polls until the profile's record names a host that
// answers as the profile's agent. It fails at once if the launched process
// exits. Otherwise it waits profileLaunchWait, and past that keeps waiting
// while the launched process is still running (it may be waiting out a
// draining predecessor), up to profileLaunchMaxWait. A nil exited channel
// means the process cannot be watched, so only the base wait applies. Every
// error carries what the agent logged since logOffset.
func waitForProfileHost(ctx context.Context, profile string, exited <-chan struct{}, logPath string, logOffset int64) (*Client, error) {
	start := time.Now()
	limit := profileLaunchWait
	if exited != nil {
		limit = max(profileLaunchWait, profileLaunchMaxWait)
	}
	for {
		c, err := resolveProfileHost(ctx, profile)
		if err == nil {
			return c, nil
		}
		select {
		case <-exited:
			return nil, fmt.Errorf("the agent for profile %q exited during startup (log: %s)%s",
				profile, logPath, logTail(logPath, logOffset))
		default:
		}
		// The exit check above ran just now, so past the base wait the
		// process is known to be alive and the cap applies.
		if time.Since(start) >= limit {
			return nil, fmt.Errorf("the agent for profile %q did not answer within %s (log: %s): %v%s",
				profile, limit, logPath, err, logTail(logPath, logOffset))
		}
		timer := time.NewTimer(profileLaunchPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-exited:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// launchProfileAgent spawns `cercano agent` for profile, detached so it
// outlives this client and serves other clients of the profile. Its output is
// appended to logPath.
func launchProfileAgent(profile, logPath string) (<-chan struct{}, error) {
	bin, err := findCercanoBinary()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", logPath, err)
	}
	// The child has its own copy of the descriptor.
	defer logFile.Close()
	if runtime.GOOS != "windows" {
		_ = logFile.Chmod(0o600)
	}
	cmd := exec.Command(bin, "agent")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// CERCANO_AUTOLAUNCHED lets the agent shut itself down when its last
	// client disconnects, as with Dial's auto-launch.
	cmd.Env = profileAgentEnv(os.Environ(), profile)
	cmd.SysProcAttr = detachSysProcAttr() // detach from the client's tty/process group
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", bin, err)
	}
	// Reap the process if it exits while this client runs, and report the
	// exit so a launch that fails (for example while the profile's previous
	// agent is still finishing) is noticed without the full wait. A healthy
	// agent keeps running after this client exits.
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	return exited, nil
}

// profileAgentEnv is env with CERCANO_PROFILE and CERCANO_AUTOLAUNCHED set
// for an agent launched for profile.
func profileAgentEnv(env []string, profile string) []string {
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(key, "CERCANO_PROFILE") || strings.EqualFold(key, "CERCANO_AUTOLAUNCHED") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "CERCANO_PROFILE="+profile, "CERCANO_AUTOLAUNCHED=1")
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// logTail formats the last lines written to path after offset for an error
// message, or returns "" when there are none.
func logTail(path string, offset int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	const maxTail = 16 << 10
	if size := fileSize(path); size-offset > maxTail {
		offset = size - maxTail
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(f, maxTail))
	if err != nil {
		return ""
	}
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > launchLogTailLines {
		lines = lines[len(lines)-launchLogTailLines:]
	}
	return "\nlast agent output:\n  " + strings.Join(lines, "\n  ")
}
