package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/internal/filelock"
	"cercano/source/server/internal/hostregistry"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/profilemigrate"
)

// modelHostShutdownTimeout bounds each phase of the model host relaunch
// (stopping the old host, the new one answering), independent of the restart
// deadline. A variable so tests can shorten it.
var modelHostShutdownTimeout = 10 * time.Second

// upgradeLaunchLockWait bounds the wait for a profile's launch lock before
// that profile's agent is restarted. A client holds it only while it launches
// the profile's agent (about agentclient's profileLaunchMaxWait), so this only
// trips if that client is stuck. A variable so tests can shorten it.
var upgradeLaunchLockWait = 30 * time.Second

// profileLaunchLockName is the per-profile lock clients hold while they check
// for and launch the profile's agent; it matches agentclient's.
const profileLaunchLockName = "launch.lock"

// profileAgent is a profile whose recorded agent answered as itself.
type profileAgent struct {
	Profile string
	Dir     string // the profile directory
	Record  hostregistry.Record
}

// upgradeRestartDeps are the effects restart-after-upgrade relies on, so the
// orchestration can be tested on any OS without a Homebrew installation.
type upgradeRestartDeps struct {
	// legacyPort is the port of a pre-profile agent; see upgradeRestartPort.
	legacyPort func() (string, error)
	// executable is the installed binary replacements are launched from.
	executable func() (string, error)
	// profileAgents lists the profiles whose agents are running.
	profileAgents func() ([]profileAgent, error)
	// restartProfile restarts the profile's agent listening at endpoint. It
	// reports false with a nil error when no agent owned by this installation
	// listens there.
	restartProfile func(ctx context.Context, executable string, agent profileAgent, endpoint netip.AddrPort) (bool, error)
	// restartAddress restarts the agent listening at endpoint and waits for
	// its replacement to listen there; used for the legacy address and
	// --address.
	restartAddress func(ctx context.Context, executable string, endpoint netip.AddrPort) (bool, error)
	// relaunchModelHost replaces a running shared model host; see
	// relaunchModelHostIn.
	relaunchModelHost func(ctx context.Context, executable string) (bool, error)
}

func runUpgradeRestart(args []string) int {
	dir, dirErr := modelhost.RunDir()
	relaunch := func(ctx context.Context, executable string) (bool, error) {
		if dirErr != nil {
			return false, dirErr
		}
		return relaunchModelHostIn(dir, func(path string) func(context.Context) error {
			return modelhost.LaunchBinary(path, dir)
		})(ctx, executable)
	}
	return upgradeRestartCommand(args, os.Stdout, upgradeRestartDeps{
		legacyPort: upgradeRestartPort,
		executable: func() (string, error) {
			path, err := os.Executable()
			if err != nil {
				return "", err
			}
			return filepath.EvalSymlinks(path)
		},
		profileAgents: runningProfileAgents,
		restartProfile: func(ctx context.Context, executable string, a profileAgent, endpoint netip.AddrPort) (bool, error) {
			return brewrestart.RestartInstalledReady(ctx, executable, endpoint, profileAgentRegistered(a.Profile, a.Dir))
		},
		restartAddress:    brewrestart.RestartInstalled,
		relaunchModelHost: relaunch,
	})
}

// runningProfileAgents lists the profiles whose agent.json names an agent
// that answers GetHostIdentity with that profile and the record's instance
// ID. A profile without a readable record, or whose recorded address is dead
// or answers as another agent, has no running agent.
func runningProfileAgents() ([]profileAgent, error) {
	if err := paths.CheckHome(); err != nil {
		return nil, err
	}
	names, err := paths.ListProfiles()
	if err != nil {
		return nil, err
	}
	var agents []profileAgent
	for _, name := range names {
		dir := paths.ProfileDir(name)
		rec, err := hostregistry.Read(dir)
		if err != nil || rec.Profile != name || rec.InstanceID == "" || !agentAnswers(name, rec) {
			continue
		}
		agents = append(agents, profileAgent{Profile: name, Dir: dir, Record: rec})
	}
	return agents, nil
}

// profileAgentRegistered returns a readiness probe for a restarted profile
// agent: the profile's agent.json names the replacement process pid, and the
// agent at the recorded address answers GetHostIdentity as that profile,
// instance and process, within ctx. A profile agent other than default
// listens on a new ephemeral port after a restart, so the old address cannot
// be watched instead. The coordinator polls it and separately fails the
// restart at once if the replacement exits.
func profileAgentRegistered(profile, dir string) func(context.Context, int) bool {
	return func(ctx context.Context, pid int) bool {
		rec, err := hostregistry.Read(dir)
		if err != nil || rec.PID != pid || rec.Profile != profile || rec.InstanceID == "" {
			return false
		}
		id, err := recordedHostIdentity(ctx, rec)
		return err == nil && id.GetProfile() == profile && id.GetInstanceId() == rec.InstanceID && int(id.GetPid()) == pid
	}
}

// upgradeRestartPort is the port of the agent to restart. Before the first
// post-upgrade start there is no profile config yet, so the agent to restart
// is a pre-profile one, listening where the legacy config said. This command
// does not migrate: the agent it relaunches does that on startup.
func upgradeRestartPort() (string, error) {
	path := config.DefaultPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return profilemigrate.LegacyPort(), nil
	}
	cfg, err := config.Load(path)
	return cfg.Port, err
}

// relaunchModelHostIn returns a function that replaces the model host
// recorded in dir with one launched from the given executable, under the
// launch lock, so agents reconnecting meanwhile attach to the installed binary
// rather than launching their own. It reports whether a host was replaced; a
// missing record, or a recorded host that no longer answers, means there is
// nothing to replace and nothing is launched.
func relaunchModelHostIn(dir string, launcher func(executable string) func(context.Context) error) func(context.Context, string) (bool, error) {
	return func(ctx context.Context, executable string) (bool, error) {
		return modelhost.Relaunch(ctx, modelhost.RelaunchOptions{
			RunDir:        dir,
			ClientVersion: version,
			Launch:        launcher(executable),
			Timeout:       modelHostShutdownTimeout,
		})
	}
}

// upgradeRestartFailureAdvice follows any failed restart.
const upgradeRestartFailureAdvice = "The update may already be installed. Check agent logs; use /restart-agent if the old agent is still serving, or start cercano manually if no agent is running."

// upgradeRestartCommand restarts every running profile's agent owned by this
// installation, one at a time, each under its profile's launch lock and with
// its own --timeout deadline. When no profile agent is running it restarts a
// pre-profile agent on the legacy address instead (the relaunched agent
// migrates on startup); --address restarts only the agent there. A failure
// is reported and the remaining agents are still restarted. With per-profile
// agent hosts off, a named profile's running agent is a leftover from a
// flag-on run and is left running, not restarted (see
// paths.PerProfileAgentsEnv). Then a running
// shared model host is replaced once, unless every attempted restart failed.
// The exit code is 1 when any restart failed.
func upgradeRestartCommand(args []string, out io.Writer, deps upgradeRestartDeps) int {
	flags := flag.NewFlagSet("restart-after-upgrade", flag.ContinueOnError)
	flags.SetOutput(out)
	address := flags.String("address", "", "explicit loopback agent address (default: every running profile's agent)")
	timeout := flags.Duration("timeout", 2*time.Minute, "deadline for each agent's drain and restart (maximum 10m)")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: cercano restart-after-upgrade [--address 127.0.0.1:PORT] [--timeout 2m]\nRestart the running agents owned by this Homebrew installation, one per profile, after a successful upgrade.\nAlso replaces a running shared model host with the installed binary.\nDoes not start an absent agent or model host, install updates, or compare versions.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *timeout <= 0 || *timeout > 10*time.Minute {
		fmt.Fprintln(out, "Invalid arguments or restart deadline.")
		return 2
	}
	var agents []profileAgent
	if *address == "" {
		var err error
		agents, err = deps.profileAgents()
		if err != nil {
			fmt.Fprintln(out, "Cannot list profiles; no restart attempted.")
			return 1
		}
		if len(agents) == 0 {
			port, err := deps.legacyPort()
			if err != nil {
				fmt.Fprintln(out, "Cannot load agent configuration; no restart attempted.")
				return 1
			}
			*address = "127.0.0.1:" + port
		}
	}
	var endpoint netip.AddrPort
	if len(agents) == 0 {
		var err error
		if endpoint, err = loopbackEndpoint(*address); err != nil {
			fmt.Fprintln(out, "Restart requires a numeric loopback address and nonzero port.")
			return 2
		}
	}
	path, err := deps.executable()
	if err != nil {
		fmt.Fprintln(out, "Cannot resolve installed executable; no restart attempted.")
		return 1
	}

	restarted, failed := 0, 0
	if len(agents) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		ok, err := deps.restartAddress(ctx, path, endpoint)
		cancel()
		switch {
		case err != nil:
			failed++
			fmt.Fprintf(out, "Upgrade restart failed: %v\n", err)
		case ok:
			restarted++
		}
	}
	for _, a := range agents {
		// With per-profile agent hosts off, a named profile's running agent
		// is a leftover from a flag-on run: the installed binary refuses to
		// serve it, so restarting would stop the old agent and fail to bring
		// up a replacement. Leave it running instead.
		if a.Profile != paths.DefaultProfile && !paths.PerProfileAgents() {
			fmt.Fprintf(out, "Left the agent for profile %q running: per-profile agent hosts are off; set %s=1 and restart to replace it.\n", a.Profile, paths.PerProfileAgentsEnv)
			continue
		}
		ok, err := restartProfileAgent(deps, path, a, *timeout)
		switch {
		case err != nil:
			failed++
			fmt.Fprintf(out, "Upgrade restart failed for profile %q: %v\n", a.Profile, err)
		case ok:
			restarted++
			fmt.Fprintf(out, "Restarted agent for profile %q.\n", a.Profile)
		default:
			fmt.Fprintf(out, "The agent for profile %q is not owned by this installation; left running.\n", a.Profile)
		}
	}

	// A running model host is replaced by the installed binary once every
	// agent has been handled, even when no agent was running. When every
	// attempted restart failed the old agents may still be serving, so the
	// model host they use is left alone. A model host failure is reported but
	// does not change the exit code.
	// It gets its own short budget (one per phase: stopping the old host, the
	// new one answering): the restarts may have spent their deadlines, and a
	// wedged host must not hold up a post-upgrade hook. Ctrl-C still cancels.
	if failed == 0 || restarted > 0 {
		sctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		sctx, scancel := context.WithTimeout(sctx, 2*modelHostShutdownTimeout)
		defer scancel()
		if relaunched, err := deps.relaunchModelHost(sctx, path); err != nil {
			fmt.Fprintf(out, "Could not restart the shared model host: %v\nIt will exit when idle; the next model request then launches the installed binary.\n", err)
		} else if relaunched {
			fmt.Fprintln(out, "Shared model host restarted using the installed binary.")
		}
	}
	switch {
	case restarted == 1:
		fmt.Fprintln(out, "Running agent restarted using the installed binary.")
	case restarted > 1:
		fmt.Fprintf(out, "%d running agents restarted using the installed binary.\n", restarted)
	case failed == 0:
		fmt.Fprintln(out, "No running agent owned by this installation; nothing started.")
	}
	if failed > 0 {
		fmt.Fprintln(out, upgradeRestartFailureAdvice)
		return 1
	}
	return 0
}

// restartProfileAgent restarts one profile's agent at its recorded address
// while holding the profile's launch lock, so a client cannot launch a
// competing agent for the profile between the old agent's exit and the
// replacement registering. The lock wait is bounded by upgradeLaunchLockWait;
// the restart itself by timeout.
func restartProfileAgent(deps upgradeRestartDeps, path string, a profileAgent, timeout time.Duration) (bool, error) {
	endpoint, err := loopbackEndpoint(a.Record.Addr)
	if err != nil {
		return false, err
	}
	lockCtx, cancel := context.WithTimeout(context.Background(), upgradeLaunchLockWait)
	release, err := filelock.Lock(lockCtx, filepath.Join(a.Dir, profileLaunchLockName))
	cancel()
	if errors.Is(err, context.DeadlineExceeded) {
		return false, fmt.Errorf("another client is still starting this profile's agent: %w", err)
	}
	if err != nil {
		return false, fmt.Errorf("take the profile's launch lock: %w", err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return deps.restartProfile(ctx, path, a, endpoint)
}

// loopbackEndpoint parses a numeric loopback address with a nonzero port.
func loopbackEndpoint(address string) (netip.AddrPort, error) {
	endpoint, err := netip.ParseAddrPort(address)
	if err != nil || !endpoint.Addr().IsLoopback() || endpoint.Port() == 0 || endpoint.Addr().Zone() != "" {
		return netip.AddrPort{}, fmt.Errorf("address %q is not a numeric loopback address with a nonzero port", address)
	}
	return endpoint, nil
}
