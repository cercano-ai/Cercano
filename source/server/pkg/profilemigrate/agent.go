package profilemigrate

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

var (
	// findListener and processExited resolve and watch the process holding
	// the legacy port; tests replace them.
	findListener  = brewrestart.ListenerProcess
	processExited = brewrestart.ProcessExited
	// exitWait bounds the wait for the stopped agent's process to exit. A
	// pre-profile agent drains in-flight turns before it closes its files.
	exitWait = 60 * time.Second
	// slowExitNotice is how long the wait runs before saying so.
	slowExitNotice = 2 * time.Second
	exitPoll       = 100 * time.Millisecond
)

// DefaultOptions returns Options for the current user that find a
// pre-profile Cercano agent on 127.0.0.1 at LegacyPort and stop it with the
// existing ShutdownAgent RPC, and that wait for other pre-profile Cercano
// processes using this home to exit. The port is resolved on first use,
// which Run does before moving anything. Nothing is ever launched.
func DefaultOptions() Options {
	var once sync.Once
	var addr string
	resolve := func() string {
		once.Do(func() { addr = net.JoinHostPort("127.0.0.1", LegacyPort()) })
		return addr
	}
	return Options{
		LegacyAgentAlive: func(ctx context.Context) (bool, error) {
			return legacyAgentAlive(ctx, resolve())
		},
		StopLegacyAgent: func(ctx context.Context) error {
			return stopAgent(ctx, resolve())
		},
		ListProcesses: listUserProcesses,
	}
}

// LegacyPort is the port a pre-profile agent listens on: CERCANO_PORT when
// set, else the port in the legacy ~/.config/cercano/config.yaml, else (on a
// retry after that file already moved) the default profile's config.yaml,
// else the default port. It panics if the home directory is unknown; see
// paths.CheckHome.
func LegacyPort() string {
	if port := os.Getenv("CERCANO_PORT"); port != "" {
		return port
	}
	for _, path := range []string{
		filepath.Join(paths.ConfigRoot(), "config.yaml"),
		filepath.Join(paths.ProfileDir(paths.DefaultProfile), "config.yaml"),
	} {
		if port, ok := configPort(path); ok {
			return port
		}
	}
	return config.Defaults().Port
}

// configPort reads the port key of a config file. ok is false when the file
// is missing, unreadable, unparseable, or has no port.
func configPort(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var raw struct {
		Port any `yaml:"port"`
	}
	if yaml.Unmarshal(data, &raw) != nil || raw.Port == nil {
		return "", false
	}
	port := fmt.Sprint(raw.Port)
	return port, port != ""
}

// probeOutcome is what the identity probe learned about the legacy port.
type probeOutcome int

const (
	probeNoListener probeOutcome = iota // connection refused
	probeAgent                          // GetConfig answered: a Cercano agent
	probeNotAgent                       // GetConfig unimplemented: another gRPC service
	probeAmbiguous                      // accepted, but no clear answer
)

// probeIdentity asks whatever listens at addr whether it is a Cercano
// agent; tests replace it.
var probeIdentity = func(ctx context.Context, addr string) probeOutcome {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		if isConnRefused(err) {
			return probeNoListener
		}
		return probeAmbiguous
	}
	conn.Close()
	c, err := agentclient.DialExisting(ctx, addr)
	if err != nil {
		return probeAmbiguous // no gRPC handshake: not speaking gRPC, or frozen
	}
	defer c.Close()
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = c.GetConfig(rctx)
	switch {
	case err == nil:
		return probeAgent
	case status.Code(err) == codes.Unimplemented:
		return probeNotAgent
	}
	return probeAmbiguous
}

// legacyAgentAlive decides whether a pre-profile agent holds addr. A clear
// probe answer decides directly. Otherwise the listening process decides:
// a cercano executable (or, once its keg was deleted, a process named
// cercano) is treated as a (possibly frozen) agent, another executable is
// not one, and an unresolvable holder fails closed.
func legacyAgentAlive(ctx context.Context, addr string) (bool, error) {
	switch probeIdentity(ctx, addr) {
	case probeNoListener, probeNotAgent:
		return false, nil
	case probeAgent:
		return true, nil
	}
	holder, found, err := lookupHolder(addr)
	if err == nil && !found && probeIdentity(ctx, addr) == probeNoListener {
		return false, nil // the listener went away meanwhile
	}
	if err != nil || !found {
		return false, fmt.Errorf("%w (something listens on %s without answering, and its process cannot be identified)", errAgentRunning, addr)
	}
	return isCercanoImage(holder.Executable, holder.ExecutableGone, holder.Name), nil
}

func lookupHolder(addr string) (brewrestart.Identity, bool, error) {
	endpoint, err := netip.ParseAddrPort(addr)
	if err != nil {
		return brewrestart.Identity{}, false, err
	}
	return findListener(endpoint)
}

// isCercanoExecutable reports whether a process image is a cercano binary.
// Both separators are accepted so a Windows path is read the same anywhere.
func isCercanoExecutable(path string) bool {
	path = strings.TrimSuffix(path, " (deleted)") // Linux, after an upgrade replaced it
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		path = path[i+1:]
	}
	return path == "cercano" || strings.EqualFold(path, "cercano.exe")
}

// stopAgent resolves the process holding addr, asks the agent to shut down
// (best effort: a frozen agent cannot answer), and, when the process is
// known, requires it to exit so no file moves while it has it open. When the
// process cannot be resolved the shutdown request must succeed, and Run then
// waits for the port to close.
func stopAgent(ctx context.Context, addr string) error {
	holder, found, err := lookupHolder(addr)
	found = found && err == nil

	shutdownErr := requestShutdown(ctx, addr)
	if found {
		if err := waitExit(ctx, holder); err != nil {
			return fmt.Errorf("%w (%v)", errAgentRunning, err)
		}
		return nil
	}
	if shutdownErr != nil {
		return fmt.Errorf("%w (shutdown request: %v)", errAgentRunning, shutdownErr)
	}
	return nil
}

func requestShutdown(ctx context.Context, addr string) error {
	c, err := agentclient.DialExisting(ctx, addr)
	if err != nil {
		return err
	}
	defer c.Close()
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.ShutdownAgent(rctx, "Cercano is moving its data into the default profile")
}

// waitExit polls until the process exits, saying once that it is waiting
// when that takes longer than slowExitNotice.
func waitExit(ctx context.Context, id brewrestart.Identity) error {
	start := time.Now()
	noticed := false
	for {
		exited, err := processExited(id)
		if err != nil {
			return fmt.Errorf("watch the previous agent (PID %d): %w", id.PID, err)
		}
		if exited {
			return nil
		}
		elapsed := time.Since(start)
		if elapsed >= exitWait {
			return fmt.Errorf("the previous agent (PID %d) did not exit within %s", id.PID, exitWait)
		}
		if !noticed && elapsed >= slowExitNotice {
			fmt.Fprintln(logOut, "Waiting for the previous Cercano agent to finish its current work…")
			noticed = true
		}
		timer := time.NewTimer(exitPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
