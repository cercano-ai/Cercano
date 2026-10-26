package profilemigrate

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
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

// stubProbe replaces the identity probe for one test.
func stubProbe(t *testing.T, outcomes ...probeOutcome) *int {
	t.Helper()
	calls := 0
	prev := probeIdentity
	probeIdentity = func(context.Context, string) probeOutcome {
		o := outcomes[min(calls, len(outcomes)-1)]
		calls++
		return o
	}
	t.Cleanup(func() { probeIdentity = prev })
	return &calls
}

func holderWithExe(exe string) func(netip.AddrPort) (brewrestart.Identity, bool, error) {
	return func(netip.AddrPort) (brewrestart.Identity, bool, error) {
		return brewrestart.Identity{PID: 4242, Executable: exe, StartSeconds: 1}, true, nil
	}
}

func holderGone(name string) func(netip.AddrPort) (brewrestart.Identity, bool, error) {
	return func(netip.AddrPort) (brewrestart.Identity, bool, error) {
		return brewrestart.Identity{PID: 4242, ExecutableGone: true, Name: name, StartSeconds: 1}, true, nil
	}
}

func neverCalledExit(t *testing.T) func(brewrestart.Identity) (bool, error) {
	return func(brewrestart.Identity) (bool, error) {
		t.Error("exit check called")
		return true, nil
	}
}

// Each probe outcome maps to one decision.
func TestLegacyAgentAliveOutcomes(t *testing.T) {
	unsupported := func(netip.AddrPort) (brewrestart.Identity, bool, error) {
		return brewrestart.Identity{}, false, errors.ErrUnsupported
	}
	for _, tt := range []struct {
		name      string
		probes    []probeOutcome
		find      func(netip.AddrPort) (brewrestart.Identity, bool, error)
		wantAlive bool
		wantErr   bool
	}{
		{"connection refused", []probeOutcome{probeNoListener}, nil, false, false},
		{"answers GetConfig", []probeOutcome{probeAgent}, nil, true, false},
		{"GetConfig unimplemented", []probeOutcome{probeNotAgent}, nil, false, false},
		{"silent cercano process", []probeOutcome{probeAmbiguous}, holderWithExe("/opt/homebrew/Cellar/cercano/0.9/bin/cercano"), true, false},
		{"silent cercano.exe", []probeOutcome{probeAmbiguous}, holderWithExe(`C:\\Cercano\\cercano.exe`), true, false},
		{"silent other process", []probeOutcome{probeAmbiguous}, holderWithExe("/usr/local/bin/some-grpc-server"), false, false},
		{"silent cercano whose keg was deleted", []probeOutcome{probeAmbiguous}, holderGone("cercano"), true, false},
		{"silent other process whose executable was deleted", []probeOutcome{probeAmbiguous}, holderGone("some-grpc-server"), false, false},
		{"silent, holder unresolvable", []probeOutcome{probeAmbiguous}, unsupported, false, true},
		{"silent, listener gone on recheck", []probeOutcome{probeAmbiguous, probeNoListener},
			func(netip.AddrPort) (brewrestart.Identity, bool, error) { return brewrestart.Identity{}, false, nil }, false, false},
		{"silent, holder not found but still listening", []probeOutcome{probeAmbiguous, probeAmbiguous},
			func(netip.AddrPort) (brewrestart.Identity, bool, error) { return brewrestart.Identity{}, false, nil }, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setHome(t)
			stubProbe(t, tt.probes...)
			find := tt.find
			if find == nil {
				find = func(netip.AddrPort) (brewrestart.Identity, bool, error) {
					t.Error("holder looked up although the probe was conclusive")
					return brewrestart.Identity{}, false, nil
				}
			}
			stubProcess(t, find, neverCalledExit(t))
			alive, err := DefaultOptions().LegacyAgentAlive(context.Background())
			if alive != tt.wantAlive || (err != nil) != tt.wantErr {
				t.Fatalf("alive = %v, err = %v; want %v, err %v", alive, err, tt.wantAlive, tt.wantErr)
			}
			if err != nil && !errors.Is(err, errAgentRunning) {
				t.Errorf("err = %v, want errAgentRunning", err)
			}
		})
	}
}

// A frozen agent: the port accepts but nothing answers. It is a cercano
// process, so the stop is best effort and its exit is required; files never
// move under it.
func TestFrozenCercanoAgentBlocksMigration(t *testing.T) {
	home, _ := setHome(t)
	ln, port := listenLoopback(t) // accepts, never speaks gRPC
	defer ln.Close()
	t.Setenv("CERCANO_PORT", port)
	stubProbe(t, probeAmbiguous)
	stubProcess(t, holderWithExe("/Applications/Cercano/cercano"), func(brewrestart.Identity) (bool, error) { return false, nil })
	exitWait = 300 * time.Millisecond
	legacy := filepath.Join(paths.ConfigRoot(), "config.yaml")
	writeFile(t, legacy, "port: \"1\"\n")

	opts := DefaultOptions()
	opts.Home = home
	err := Run(context.Background(), opts)
	if !errors.Is(err, errAgentRunning) {
		t.Fatalf("Run err = %v, want errAgentRunning", err)
	}
	if !exists(legacy) || exists(markerPath()) {
		t.Error("migrated under a frozen agent")
	}
}

func TestFrozenCercanoAgentMigratesOnceItExits(t *testing.T) {
	home, _ := setHome(t)
	ln, port := listenLoopback(t)
	t.Setenv("CERCANO_PORT", port)
	stubProbe(t, probeAmbiguous, probeNoListener)
	polls := 0
	stubProcess(t, holderWithExe("/Applications/Cercano/cercano"), func(brewrestart.Identity) (bool, error) {
		polls++
		if polls == 3 {
			ln.Close() // the process exits and its listener goes with it
			return true, nil
		}
		return false, nil
	})
	writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: \"1\"\n")
	opts := DefaultOptions()
	opts.Home = home
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !exists(markerPath()) {
		t.Fatal("marker not written")
	}
}

// ShutdownAgent unimplemented after GetConfig answered means an older or
// skewed Cercano agent, not a stranger: it must still exit before files move.
func TestShutdownUnimplementedStillRequiresExit(t *testing.T) {
	t.Run("PID resolved", func(t *testing.T) {
		setHome(t)
		serveOnPort(t, func(s *grpc.Server) { proto.RegisterAgentServer(s, configOnlyAgent{}) })
		stubProcess(t, agentIdentity, func(brewrestart.Identity) (bool, error) { return false, nil })
		exitWait = 200 * time.Millisecond
		err := DefaultOptions().StopLegacyAgent(context.Background())
		if !errors.Is(err, errAgentRunning) {
			t.Fatalf("err = %v, want errAgentRunning", err)
		}
	})
	t.Run("PID unresolvable", func(t *testing.T) {
		setHome(t)
		serveOnPort(t, func(s *grpc.Server) { proto.RegisterAgentServer(s, configOnlyAgent{}) })
		stubProcess(t, func(netip.AddrPort) (brewrestart.Identity, bool, error) {
			return brewrestart.Identity{}, false, errors.ErrUnsupported
		}, neverCalledExit(t))
		err := DefaultOptions().StopLegacyAgent(context.Background())
		if !errors.Is(err, errAgentRunning) {
			t.Fatalf("err = %v, want errAgentRunning", err)
		}
	})
}

// The real probe against real listeners.
func TestProbeIdentity(t *testing.T) {
	ctx := context.Background()
	t.Run("closed port", func(t *testing.T) {
		ln, port := listenLoopback(t)
		ln.Close()
		if got := probeIdentity(ctx, net.JoinHostPort("127.0.0.1", port)); got != probeNoListener {
			t.Errorf("probe = %v, want probeNoListener", got)
		}
	})
	t.Run("not gRPC", func(t *testing.T) {
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
		if got := probeIdentity(ctx, net.JoinHostPort("127.0.0.1", port)); got != probeAmbiguous {
			t.Errorf("probe = %v, want probeAmbiguous", got)
		}
	})
	t.Run("other gRPC service", func(t *testing.T) {
		setHome(t)
		serveOnPort(t, func(*grpc.Server) {})
		if got := probeIdentity(ctx, net.JoinHostPort("127.0.0.1", os.Getenv("CERCANO_PORT"))); got != probeNotAgent {
			t.Errorf("probe = %v, want probeNotAgent", got)
		}
	})
	t.Run("agent", func(t *testing.T) {
		setHome(t)
		serveOnPort(t, func(s *grpc.Server) { proto.RegisterAgentServer(s, configOnlyAgent{}) })
		if got := probeIdentity(ctx, net.JoinHostPort("127.0.0.1", os.Getenv("CERCANO_PORT"))); got != probeAgent {
			t.Errorf("probe = %v, want probeAgent", got)
		}
	})
}

func TestDefaultOptionsIgnoreNonAgents(t *testing.T) {
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
