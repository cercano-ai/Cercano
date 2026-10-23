package profilemigrate

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/proto"

	"google.golang.org/grpc"
)

func TestLegacyAgentAddr(t *testing.T) {
	t.Run("default port", func(t *testing.T) {
		setHome(t)
		if got := legacyAgentAddr(); got != "127.0.0.1:50052" {
			t.Errorf("addr = %q", got)
		}
	})
	t.Run("legacy config port", func(t *testing.T) {
		setHome(t)
		writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: \"50111\"\n")
		// The profile's config is not the legacy one and must not be read.
		writeFile(t, filepath.Join(paths.ProfileDir("default"), "config.yaml"), "port: \"50999\"\n")
		if got := legacyAgentAddr(); got != "127.0.0.1:50111" {
			t.Errorf("addr = %q", got)
		}
	})
	t.Run("numeric legacy port", func(t *testing.T) {
		setHome(t)
		writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: 50112\n")
		if got := legacyAgentAddr(); got != "127.0.0.1:50112" {
			t.Errorf("addr = %q", got)
		}
	})
	t.Run("CERCANO_PORT wins", func(t *testing.T) {
		setHome(t)
		writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: \"50111\"\n")
		t.Setenv("CERCANO_PORT", "50222")
		if got := legacyAgentAddr(); got != "127.0.0.1:50222" {
			t.Errorf("addr = %q", got)
		}
	})
}

type shutdownRecorder struct {
	proto.UnimplementedAgentServer
	reasons chan string
}

func (s *shutdownRecorder) ShutdownAgent(_ context.Context, req *proto.ShutdownAgentRequest) (*proto.ShutdownAgentResponse, error) {
	s.reasons <- req.GetReason()
	return &proto.ShutdownAgentResponse{Accepted: true}, nil
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

func TestDefaultOptionsStopAnAnsweringAgent(t *testing.T) {
	setHome(t)
	ln, port := listenLoopback(t)
	t.Setenv("CERCANO_PORT", port)
	rec := &shutdownRecorder{reasons: make(chan string, 1)}
	srv := grpc.NewServer()
	proto.RegisterAgentServer(srv, rec)
	go srv.Serve(ln)
	defer srv.Stop()

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
	case reason := <-rec.reasons:
		if reason == "" {
			t.Error("shutdown requested without a reason")
		}
	case <-ctx.Done():
		t.Fatal("ShutdownAgent never reached the agent")
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
}
