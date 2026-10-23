package profilemigrate

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/proto"

	"google.golang.org/grpc"
)

func TestLegacyPort(t *testing.T) {
	t.Run("default port", func(t *testing.T) {
		setHome(t)
		if got := LegacyPort(); got != "50052" {
			t.Errorf("port = %q", got)
		}
	})
	t.Run("legacy config port", func(t *testing.T) {
		setHome(t)
		writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: \"50111\"\n")
		// While the legacy file exists, the profile's config is not consulted.
		writeFile(t, filepath.Join(paths.ProfileDir("default"), "config.yaml"), "port: \"50999\"\n")
		if got := LegacyPort(); got != "50111" {
			t.Errorf("port = %q", got)
		}
	})
	t.Run("numeric legacy port", func(t *testing.T) {
		setHome(t)
		writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: 50112\n")
		if got := LegacyPort(); got != "50112" {
			t.Errorf("port = %q", got)
		}
	})
	t.Run("retry after config.yaml moved", func(t *testing.T) {
		setHome(t)
		writeFile(t, filepath.Join(paths.ProfileDir("default"), "config.yaml"), "port: \"50113\"\n")
		if got := LegacyPort(); got != "50113" {
			t.Errorf("port = %q", got)
		}
	})
	t.Run("CERCANO_PORT wins", func(t *testing.T) {
		setHome(t)
		writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: \"50111\"\n")
		t.Setenv("CERCANO_PORT", "50222")
		if got := LegacyPort(); got != "50222" {
			t.Errorf("port = %q", got)
		}
	})
}

// fakeAgent answers GetConfig (so it identifies as a Cercano agent) and,
// unless noShutdown, ShutdownAgent.
type fakeAgent struct {
	proto.UnimplementedAgentServer
	shutdowns chan string
}

func (a *fakeAgent) GetConfig(context.Context, *proto.GetConfigRequest) (*proto.GetConfigResponse, error) {
	return &proto.GetConfigResponse{}, nil
}

func (a *fakeAgent) ShutdownAgent(_ context.Context, req *proto.ShutdownAgentRequest) (*proto.ShutdownAgentResponse, error) {
	a.shutdowns <- req.GetReason()
	return &proto.ShutdownAgentResponse{Accepted: true}, nil
}

// configOnlyAgent identifies as an agent but has no ShutdownAgent.
type configOnlyAgent struct{ proto.UnimplementedAgentServer }

func (configOnlyAgent) GetConfig(context.Context, *proto.GetConfigRequest) (*proto.GetConfigResponse, error) {
	return &proto.GetConfigResponse{}, nil
}

func listenLoopback(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return ln, port
}

// serveOnPort starts a gRPC server on a fresh loopback port, points
// CERCANO_PORT at it and returns the server.
func serveOnPort(t *testing.T, register func(*grpc.Server)) *grpc.Server {
	t.Helper()
	ln, port := listenLoopback(t)
	t.Setenv("CERCANO_PORT", port)
	srv := grpc.NewServer()
	register(srv)
	go srv.Serve(ln)
	t.Cleanup(srv.Stop)
	return srv
}

// stubProcess replaces the PID lookup and exit check for one test.
func stubProcess(t *testing.T, find func(netip.AddrPort) (brewrestart.Identity, bool, error), exited func(brewrestart.Identity) (bool, error)) {
	t.Helper()
	prevFind, prevExited := findListener, processExited
	prevWait, prevNotice, prevPoll := exitWait, slowExitNotice, exitPoll
	findListener, processExited = find, exited
	exitWait, slowExitNotice, exitPoll = 2*time.Second, 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() {
		findListener, processExited = prevFind, prevExited
		exitWait, slowExitNotice, exitPoll = prevWait, prevNotice, prevPoll
	})
}

func agentIdentity(netip.AddrPort) (brewrestart.Identity, bool, error) {
	return brewrestart.Identity{PID: 4242, StartSeconds: 1}, true, nil
}

func TestDefaultOptionsStopAnAnsweringAgent(t *testing.T) {
	setHome(t)
	rec := &fakeAgent{shutdowns: make(chan string, 1)}
	serveOnPort(t, func(s *grpc.Server) { proto.RegisterAgentServer(s, rec) })
	stubProcess(t, agentIdentity, func(brewrestart.Identity) (bool, error) { return true, nil })

	opts := DefaultOptions()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	alive, err := opts.LegacyAgentAlive(ctx)
	if err != nil || !alive {
		t.Fatalf("alive = %v, %v; want true", alive, err)
	}
	if err := opts.StopLegacyAgent(ctx); err != nil {
		t.Fatalf("StopLegacyAgent: %v", err)
	}
	select {
	case reason := <-rec.shutdowns:
		if reason == "" {
			t.Error("shutdown requested without a reason")
		}
	case <-ctx.Done():
		t.Fatal("ShutdownAgent never reached the agent")
	}
}

// The listener's PID is resolved before ShutdownAgent, and the stop waits
// for that process to exit (not just for the port to close), saying so once
// when it takes a while.
func TestStopWaitsForAgentProcessExit(t *testing.T) {
	_, out := setHome(t)
	rec := &fakeAgent{shutdowns: make(chan string, 1)}
	serveOnPort(t, func(s *grpc.Server) { proto.RegisterAgentServer(s, rec) })
	var resolvedBeforeShutdown atomic.Bool
	var polls atomic.Int32
	stubProcess(t,
		func(netip.AddrPort) (brewrestart.Identity, bool, error) {
			resolvedBeforeShutdown.Store(len(rec.shutdowns) == 0)
			return brewrestart.Identity{PID: 4242, StartSeconds: 1}, true, nil
		},
		func(id brewrestart.Identity) (bool, error) {
			if id.PID != 4242 {
				t.Errorf("waited on PID %d", id.PID)
			}
			return polls.Add(1) > 15, nil // ~150ms of draining
		})

	if err := DefaultOptions().StopLegacyAgent(context.Background()); err != nil {
		t.Fatalf("StopLegacyAgent: %v", err)
	}
	if !resolvedBeforeShutdown.Load() {
		t.Error("PID resolved after ShutdownAgent")
	}
	if polls.Load() <= 15 {
		t.Errorf("returned before the process exited (%d polls)", polls.Load())
	}
	if got := out.String(); strings.Count(got, "Waiting for the previous Cercano agent to finish its current work") != 1 {
		t.Errorf("stderr = %q, want the waiting line once", got)
	}
}

func TestStopFailsWhenAgentNeverExits(t *testing.T) {
	home, _ := setHome(t)
	rec := &fakeAgent{shutdowns: make(chan string, 1)}
	serveOnPort(t, func(s *grpc.Server) { proto.RegisterAgentServer(s, rec) })
	stubProcess(t, agentIdentity, func(brewrestart.Identity) (bool, error) { return false, nil })
	exitWait = 300 * time.Millisecond
	legacy := filepath.Join(paths.ConfigRoot(), "config.yaml")
	writeFile(t, legacy, "port: \"1\"\n")

	opts := DefaultOptions()
	opts.Home = home
	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "quit Cercano") {
		t.Fatalf("Run err = %v, want a quit-and-retry error", err)
	}
	if !exists(legacy) || exists(filepath.Join(paths.ProfileDir("default"), "config.yaml")) {
		t.Error("files moved although the agent never exited")
	}
	if exists(markerPath()) {
		t.Error("marker written although the agent never exited")
	}
}

func TestStopFallsBackWithoutPIDLookup(t *testing.T) {
	setHome(t)
	rec := &fakeAgent{shutdowns: make(chan string, 1)}
	serveOnPort(t, func(s *grpc.Server) { proto.RegisterAgentServer(s, rec) })
	stubProcess(t,
		func(netip.AddrPort) (brewrestart.Identity, bool, error) {
			return brewrestart.Identity{}, false, errors.ErrUnsupported
		},
		func(brewrestart.Identity) (bool, error) {
			t.Error("exit check without a PID")
			return true, nil
		})
	if err := DefaultOptions().StopLegacyAgent(context.Background()); err != nil {
		t.Fatalf("StopLegacyAgent: %v", err)
	}
	if len(rec.shutdowns) != 1 {
		t.Fatal("ShutdownAgent not sent")
	}
}

func TestShutdownUnimplementedIsNotAnAgent(t *testing.T) {
	setHome(t)
	serveOnPort(t, func(s *grpc.Server) { proto.RegisterAgentServer(s, configOnlyAgent{}) })
	stubProcess(t, agentIdentity, func(brewrestart.Identity) (bool, error) { return false, nil })
	err := DefaultOptions().StopLegacyAgent(context.Background())
	if !errors.Is(err, ErrNotAgent) {
		t.Fatalf("err = %v, want ErrNotAgent", err)
	}
}

func TestDefaultOptionsIgnoreNonAgents(t *testing.T) {
	t.Run("closed port", func(t *testing.T) {
		setHome(t)
		ln, port := listenLoopback(t)
		ln.Close()
		t.Setenv("CERCANO_PORT", port)
		alive, err := DefaultOptions().LegacyAgentAlive(context.Background())
		if err != nil || alive {
			t.Fatalf("alive = %v, %v; want false", alive, err)
		}
	})
	t.Run("not gRPC", func(t *testing.T) {
		setHome(t)
		ln, port := listenLoopback(t)
		defer ln.Close()
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				c.Write([]byte("hello\n"))
				c.Close()
			}
		}()
		t.Setenv("CERCANO_PORT", port)
		alive, err := DefaultOptions().LegacyAgentAlive(context.Background())
		if err != nil || alive {
			t.Fatalf("alive = %v, %v; want false", alive, err)
		}
	})
	t.Run("other gRPC service", func(t *testing.T) {
		home, _ := setHome(t)
		serveOnPort(t, func(*grpc.Server) {}) // speaks gRPC, has no Agent service
		alive, err := DefaultOptions().LegacyAgentAlive(context.Background())
		if err != nil || alive {
			t.Fatalf("alive = %v, %v; want false", alive, err)
		}
		// And it never blocks the migration.
		writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "ollama_url: x\n")
		opts := DefaultOptions()
		opts.Home = home
		if err := Run(context.Background(), opts); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !exists(markerPath()) {
			t.Error("marker not written")
		}
	})
}
