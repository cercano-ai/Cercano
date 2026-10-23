package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/pkg/config"
)

// modelHostShutdownTimeout bounds each phase of the model host relaunch
// (stopping the old host, the new one answering), independent of the restart
// deadline. A variable so tests can shorten it.
var modelHostShutdownTimeout = 10 * time.Second

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
	return upgradeRestartCommand(args, os.Stdout,
		func() (string, error) { cfg, err := config.Load(config.DefaultPath()); return cfg.Port, err },
		func() (string, error) {
			path, err := os.Executable()
			if err != nil {
				return "", err
			}
			return filepath.EvalSymlinks(path)
		},
		relaunch,
		brewrestart.RestartInstalled)
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

func upgradeRestartCommand(args []string, out io.Writer, configuredPort func() (string, error), executable func() (string, error), relaunchModelHost func(context.Context, string) (bool, error), restart func(context.Context, string, netip.AddrPort) (bool, error)) int {
	flags := flag.NewFlagSet("restart-after-upgrade", flag.ContinueOnError)
	flags.SetOutput(out)
	address := flags.String("address", "", "explicit loopback agent address (default: configured port)")
	timeout := flags.Duration("timeout", 2*time.Minute, "deadline for drain and restart (maximum 10m)")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: cercano restart-after-upgrade [--address 127.0.0.1:PORT] [--timeout 2m]\nRestart an existing agent owned by this Homebrew installation after a successful upgrade.\nAlso replaces a running shared model host with the installed binary.\nDoes not start an absent agent or model host, install updates, or compare versions.")
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
	if *address == "" {
		port, err := configuredPort()
		if err != nil {
			fmt.Fprintln(out, "Cannot load agent configuration; no restart attempted.")
			return 1
		}
		*address = "127.0.0.1:" + port
	}
	endpoint, err := netip.ParseAddrPort(*address)
	if err != nil || !endpoint.Addr().IsLoopback() || endpoint.Port() == 0 || endpoint.Addr().Zone() != "" {
		fmt.Fprintln(out, "Restart requires a numeric loopback address and nonzero port.")
		return 2
	}
	path, err := executable()
	if err != nil {
		fmt.Fprintln(out, "Cannot resolve installed executable; no restart attempted.")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	restarted, err := restart(ctx, path, endpoint)
	if err != nil {
		fmt.Fprintf(out, "Upgrade restart failed: %v\nThe update may already be installed. Check agent logs; use /restart-agent if the old agent is still serving, or start cercano manually if no agent is running.\n", err)
		return 1
	}
	// A running model host is replaced by the installed binary even when no
	// agent was running. A failure here is reported but does not change the
	// exit code: the agent restart itself succeeded.
	// It gets its own short budget (one per phase: stopping the old host, the
	// new one answering): the restart may have spent most of ctx, and a wedged
	// host must not hold up a post-upgrade hook. Ctrl-C still cancels.
	sctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	sctx, scancel := context.WithTimeout(sctx, 2*modelHostShutdownTimeout)
	defer scancel()
	if relaunched, err := relaunchModelHost(sctx, path); err != nil {
		fmt.Fprintf(out, "Could not restart the shared model host: %v\nIt will exit when idle; the next model request then launches the installed binary.\n", err)
	} else if relaunched {
		fmt.Fprintln(out, "Shared model host restarted using the installed binary.")
	}
	if restarted {
		fmt.Fprintln(out, "Running agent restarted using the installed binary.")
	} else {
		fmt.Fprintln(out, "No running agent owned by this installation; nothing started.")
	}
	return 0
}
