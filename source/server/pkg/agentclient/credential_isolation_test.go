package agentclient

import (
	"context"
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

func TestCredentialIsolationRejectsOldAgent(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	proto.RegisterAgentServer(server, &proto.UnimplementedAgentServer{})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	for _, dial := range []struct {
		name string
		call func(context.Context, string) (*Client, error)
	}{{"Dial", Dial}, {"DialExisting", DialExisting}} {
		t.Run(dial.name, func(t *testing.T) {
			c, err := dial.call(context.Background(), listener.Addr().String())
			if c != nil {
				c.Close()
			}
			if err == nil {
				t.Fatal("accepted old credential-enabled/unknown Agent")
			}
		})
	}
}

type isolationFake struct {
	proto.UnimplementedAgentServer
	disabled bool
	err      error
	stall    bool
}

func (f *isolationFake) GetCredentialPolicy(ctx context.Context, _ *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	if f.stall {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: f.disabled}, f.err
}

type connectionCounter struct{ active atomic.Int32 }

func (c *connectionCounter) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}
func (c *connectionCounter) HandleRPC(context.Context, stats.RPCStats) {}
func (c *connectionCounter) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}
func (c *connectionCounter) HandleConn(_ context.Context, s stats.ConnStats) {
	switch s.(type) {
	case *stats.ConnBegin:
		c.active.Add(1)
	case *stats.ConnEnd:
		c.active.Add(-1)
	}
}

func TestCredentialIsolationPaths(t *testing.T) {
	for _, tc := range []struct {
		name, setting string
		fake          isolationFake
		accept        bool
	}{
		{"false", "1", isolationFake{}, false},
		{"true", "1", isolationFake{disabled: true}, true},
		{"error", "1", isolationFake{err: status.Error(codes.Internal, "fixture")}, false},
		{"unimplemented", "1", isolationFake{err: status.Error(codes.Unimplemented, "old")}, false},
		{"deadline", "1", isolationFake{stall: true}, false},
		{"malformed", "invalid", isolationFake{}, false},
		{"enabled unchanged", "0", isolationFake{}, true},
		{"default unchanged", "absent", isolationFake{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", tc.setting)
			if tc.setting == "absent" {
				os.Unsetenv("CERCANO_DISABLE_CREDENTIALS")
			}
			t.Setenv("TMPDIR", t.TempDir())
			var health, other atomic.Int32
			counter := &connectionCounter{}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer(grpc.StatsHandler(counter), grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
				if info.FullMethod == proto.Agent_GetCredentialPolicy_FullMethodName {
					health.Add(1)
				} else {
					other.Add(1)
				}
				return next(ctx, req)
			}))
			proto.RegisterAgentServer(server, &tc.fake)
			go server.Serve(listener)
			defer server.Stop()
			addr := listener.Addr().String()
			for _, path := range []string{"Dial", "DialExisting", "recheck", "reconnect"} {
				t.Run(path, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
					defer cancel()
					var c *Client
					var err error
					switch path {
					case "Dial":
						c, err = Dial(ctx, addr)
					case "DialExisting":
						c, err = DialExisting(ctx, addr)
					case "recheck":
						_, _, err = ensureServerLaunched(ctx, addr, time.Second)
					case "reconnect":
						c = &Client{addr: addr, stopWatch: make(chan struct{}), state: ConnStateReconnecting}
						err = c.reconnect(ctx)
					}
					if c != nil {
						defer c.Close()
					}
					if tc.accept && err != nil {
						t.Fatal(err)
					}
					if !tc.accept && err == nil {
						t.Fatal("unsafe connection accepted")
					}
					if !tc.accept && c != nil && c.readConn() != nil {
						t.Fatal("refused connection exposed")
					}
					if !tc.accept && !errors.Is(err, ErrCredentialIsolation) {
						t.Fatalf("wrong refusal: %v", err)
					}
				})
			}
			if other.Load() != 0 {
				t.Fatalf("sent %d non-health RPCs (reset/tool/credential mutation)", other.Load())
			}
			if (tc.setting == "0" || tc.setting == "absent") && health.Load() != 0 {
				t.Fatal("enabled client performed handshake")
			}
			deadline := time.Now().Add(time.Second)
			for counter.active.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if counter.active.Load() != 0 {
				t.Fatal("leaked connection")
			}
		})
	}
}

// A fake executable is the only launcher reachable in this test. Malformed
// settings must reject even when there is no listening host to attest.
func TestCredentialIsolationMalformedNeverLaunches(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	root := t.TempDir()
	marker := filepath.Join(root, "launched")
	if err := os.WriteFile(filepath.Join(root, "cercano"), []byte("#!/bin/sh\n: > '"+marker+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	t.Setenv("TMPDIR", root)
	oldName := cercanoBinaryName
	cercanoBinaryName = "nonexistent-policy-fixture"
	defer func() { cercanoBinaryName = oldName }()
	for _, value := range []string{"invalid", "", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", value)
			ctx := context.Background()
			if c, err := Dial(ctx, "127.0.0.1:0"); c != nil || !errors.Is(err, ErrCredentialIsolation) {
				t.Fatalf("Dial: %v %v", c, err)
			}
			if _, _, err := ensureServerLaunched(ctx, "127.0.0.1:0", time.Millisecond); !errors.Is(err, ErrCredentialIsolation) {
				t.Fatal(err)
			}
			if _, err := autoLaunchServer("127.0.0.1:0"); !errors.Is(err, ErrCredentialIsolation) {
				t.Fatal(err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("launcher executed: %v", err)
			}
		})
	}
}
