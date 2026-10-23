package profilemigrate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
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

// ErrNotAgent means the listener on the legacy port is not a Cercano agent.
// Run then migrates without stopping anything.
var ErrNotAgent = errors.New("the listener on the legacy port is not a Cercano agent")

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
// existing ShutdownAgent RPC. The port is resolved on first use, which Run
// does before moving anything. Nothing is ever launched.
func DefaultOptions() Options {
	var once sync.Once
	var addr string
	resolve := func() string {
		once.Do(func() { addr = net.JoinHostPort("127.0.0.1", LegacyPort()) })
		return addr
	}
	return Options{
		LegacyAgentAlive: func(ctx context.Context) (bool, error) {
			return agentAnswering(ctx, resolve()), nil
		},
		StopLegacyAgent: func(ctx context.Context) error {
			return stopAgent(ctx, resolve())
		},
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

// agentAnswering reports whether a Cercano agent answers at addr: the port
// accepts a connection, speaks gRPC, and serves the agent's GetConfig RPC. A
// closed port, a listener that is not gRPC, or another gRPC service is not
// one.
func agentAnswering(ctx context.Context, addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	c, err := agentclient.DialExisting(ctx, addr)
	if err != nil {
		return false
	}
	defer c.Close()
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = c.GetConfig(rctx)
	return err == nil
}

// stopAgent resolves the process holding addr, asks the agent to shut down,
// and waits for that process to exit, so no file is moved while the agent
// still has it open. Where the process cannot be resolved it returns after
// the shutdown request and Run falls back to waiting for the port to close.
func stopAgent(ctx context.Context, addr string) error {
	var holder brewrestart.Identity
	found := false
	if endpoint, err := netip.ParseAddrPort(addr); err == nil {
		if id, ok, err := findListener(endpoint); err == nil && ok {
			holder, found = id, true
		}
	}

	c, err := agentclient.DialExisting(ctx, addr)
	if err != nil {
		return err
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = c.ShutdownAgent(rctx, "Cercano is moving its data into the default profile")
	cancel()
	c.Close()
	if status.Code(err) == codes.Unimplemented {
		return fmt.Errorf("%w: %v", ErrNotAgent, err)
	}
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	return waitExit(ctx, holder)
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
