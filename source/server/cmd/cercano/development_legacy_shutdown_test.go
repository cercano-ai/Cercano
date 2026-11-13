package main

import (
	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/pkg/devhostauth"
	"context"
	"crypto/sha256"
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
			adapter := newDevelopmentContainedSource(state, destination)
			adapter.inspect = func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error) {
				return brewrestart.Identity{PID: 42, StartSeconds: 1, Executable: filepath.Join(generation, "cercano")}, nil
			}
			signals := 0
			adapter.terminate = func(ctx context.Context, host string, endpoint netip.AddrPort, expected brewrestart.Identity, validate func() error) error {
				if err := validate(); err != nil {
					return err
				}
				signals++
				return nil
			}
			checks := 0
			adapter.absent = func(context.Context, string) error {
				checks++
				if err := devFlock(lock, devLockExclusive|devLockNonBlock); err == nil {
					t.Fatal("launch exclusion lost")
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
			err = adapter.stopAndUseSource(ctx, id, snapshot, func() error { return nil }, func(context.Context) error { used = true; return nil })
			if wrongToken {
				if err == nil || used || fixture.requests.Load() != 0 || signals != 0 {
					t.Fatalf("unauthenticated stop: %v", err)
				}
			} else if err != nil || !used || fixture.requests.Load() != 0 || signals != 1 || checks != 1 {
				t.Fatalf("shutdown not verified: %v checks=%d", err, checks)
			}
		})
	}
}
