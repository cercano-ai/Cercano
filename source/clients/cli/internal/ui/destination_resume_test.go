package ui

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type blockedRecoveryFixture struct{ deadOwnerFixture }

func (s *blockedRecoveryFixture) LifecycleRecover(context.Context, *pb.LifecycleRecoverRequest) (*pb.LifecycleControlResponse, error) {
	return nil, status.Error(codes.FailedPrecondition, "source death not verified: source host still has a live process; safe live-owner fencing unavailable")
}

func TestResumeNeverCallsLiveLegacySource(t *testing.T) {
	root := t.TempDir()
	var calls atomic.Int64
	serve := func(host string, fixture pb.AgentServer, historical bool) DevHostSelection {
		t.Helper()
		token := filepath.Join(root, host+".token")
		if err := devhostauth.Create(token); err != nil {
			t.Fatal(err)
		}
		auth, err := devhostauth.Load(host, token)
		if err != nil {
			t.Fatal(err)
		}
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		opts := auth.ServerOptions()
		if historical {
			opts = append(opts, grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
				calls.Add(1)
				return handler(ctx, req)
			}))
		}
		g := grpc.NewServer(opts...)
		pb.RegisterAgentServer(g, fixture)
		go g.Serve(lis)
		t.Cleanup(g.Stop)
		return DevHostSelection{Identity: &pb.LifecycleIdentity{Host: host, Build: "fixture", Contract: "fixture", Schema: 2, Store: filepath.Join(root, "conversations.db")}, Endpoint: lis.Addr().String(), TokenFile: token}
	}
	// A is reachable but deliberately has no LifecycleForceRelinquish RPC.
	source := serve("A", &pb.UnimplementedAgentServer{}, true)
	dest := serve("B", &blockedRecoveryFixture{}, false)
	hosts := filepath.Join(root, "hosts")
	if err := os.Mkdir(hosts, 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(source)
	if err := os.WriteFile(filepath.Join(hosts, "A.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	selected, err := reopenDevHost(context.Background(), dest, "history")
	if calls.Load() != 0 {
		t.Fatalf("ordinary resume dialed historical legacy source: %d RPC(s), result: %v", calls.Load(), err)
	}
	if selected.Identity.Host != "B" || err == nil || !strings.Contains(err.Error(), "source death not verified") {
		t.Fatalf("lost destination blocker: %+v %v", selected, err)
	}
}
