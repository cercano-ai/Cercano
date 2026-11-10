package main

import (
	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/pkg/devhostauth"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	pb "cercano/source/server/pkg/proto"
)

func TestLegacyAuthenticatedShutdownWait(t *testing.T) {
	for _, mode := range []string{"bounded-success", "drain-hangs", "declined", "response-loss", "refused", "cancelled", "unreadable-process"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			requests, checks := 0, 0
			err := awaitLegacyShutdown(ctx, func() error {
				if mode == "declined" {
					return fmt.Errorf("consent declined")
				}
				return nil
			}, func(context.Context) (*pb.ShutdownAgentResponse, error) {
				requests++
				if mode == "response-loss" {
					return nil, fmt.Errorf("response lost")
				}
				return &pb.ShutdownAgentResponse{Accepted: mode != "refused"}, nil
			}, func(context.Context) error {
				checks++
				if mode == "bounded-success" && checks >= 2 {
					return nil
				}
				return fmt.Errorf("source live or absence unverifiable")
			})
			if (err == nil) != (mode == "bounded-success") {
				t.Fatalf("err=%v", err)
			}
			if mode == "bounded-success" && checks < 2 {
				t.Fatal("acceptance treated as completion")
			}
			if (mode == "declined" || mode == "cancelled") && requests != 0 {
				t.Fatal("unapproved shutdown")
			}
			if (mode == "response-loss" || mode == "refused") && checks != 0 {
				t.Fatal("uncertain/refused request authorized continuation")
			}
			if (mode == "drain-hangs" || mode == "unreadable-process") && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("not bounded: %v", err)
			}
		})
	}
}

type legacyShutdownRPCFixture struct {
	developmentIdentityFake
	requests atomic.Int32
}

func (f *legacyShutdownRPCFixture) ShutdownAgent(context.Context, *pb.ShutdownAgentRequest) (*pb.ShutdownAgentResponse, error) {
	f.requests.Add(1)
	return &pb.ShutdownAgentResponse{Accepted: true}, nil
}

func TestLegacyShutdownAuthenticatedTransport(t *testing.T) {
	for _, wrongToken := range []bool{false, true} {
		t.Run(fmt.Sprint(wrongToken), func(t *testing.T) {
			state, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			hosts := filepath.Join(state, "hosts")
			if err := os.Mkdir(hosts, 0700); err != nil {
				t.Fatal(err)
			}
			generation := filepath.Join(state, "generation")
			base := filepath.Join(hosts, fmt.Sprintf("%x", sha256.Sum256([]byte(generation))))
			token := base + ".token"
			if err := devhostauth.Create(token); err != nil {
				t.Fatal(err)
			}
			auth, err := devhostauth.Load("source", token)
			if err != nil {
				t.Fatal(err)
			}
			id := &pb.LifecycleIdentity{Host: "source", Build: "old", Store: "store", Contract: "contract", Schema: 2}
			destination := &pb.LifecycleIdentity{Host: "destination", Store: "store", Contract: "contract", Schema: 2}
			fixture := &legacyShutdownRPCFixture{developmentIdentityFake: developmentIdentityFake{identity: id}}
			server := grpc.NewServer(auth.ServerOptions()...)
			defer server.Stop()
			pb.RegisterAgentServer(server, fixture)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			go server.Serve(listener)
			if wrongToken {
				token = base + ".wrong"
				if err := devhostauth.Create(token); err != nil {
					t.Fatal(err)
				}
			}
			for path, value := range map[string]any{
				base + ".json":     launchSelection{Identity: id, Endpoint: listener.Addr().String(), TokenFile: token},
				base + ".starting": map[string]string{"host": "source", "generation": generation},
			} {
				if err := launchExclusive(path, value); err != nil {
					t.Fatal(err)
				}
			}
			lock, err := os.OpenFile(base+".lock", os.O_CREATE|os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			adapter := newDevelopmentLegacyStop(state, destination)
			adapter.inspect = func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error) {
				return brewrestart.Identity{PID: 42, StartSeconds: 1, Executable: filepath.Join(generation, "cercano")}, nil
			}
			checks := 0
			adapter.absent = func(context.Context, string) error {
				checks++
				if err := devFlock(lock, devLockExclusive|devLockNonBlock); err == nil {
					t.Fatal("launch exclusion lost")
				}
				if checks == 1 {
					return fmt.Errorf("draining")
				}
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			snapshot, err := adapter.Prepare(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			used := false
			err = adapter.withStoppedSource(ctx, id, snapshot, func() error { return nil }, func(context.Context) error { used = true; return nil })
			if wrongToken {
				if err == nil || used || fixture.requests.Load() != 0 {
					t.Fatalf("unauthenticated stop: %v", err)
				}
			} else if err != nil || !used || fixture.requests.Load() != 1 || checks < 3 {
				t.Fatalf("shutdown not verified: %v checks=%d", err, checks)
			}
		})
	}
}
