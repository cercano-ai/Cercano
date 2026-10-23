package agentclient

import (
	"context"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// Local inert backends only: no real agent, credentials, or shutdown RPC.
func TestPinnedDrainReplacement(t *testing.T) {
	for _, setting := range []string{"absent", "0", "1"} {
		t.Run(setting, func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", setting)
			if setting == "absent" {
				os.Unsetenv("CERCANO_DISABLE_CREDENTIALS")
			}
			var drains atomic.Int32
			start := func(addr string) (*grpc.Server, string) {
				l, err := net.Listen("tcp", addr)
				if err != nil {
					t.Fatal(err)
				}
				s := grpc.NewServer(grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
					if info.FullMethod == proto.Agent_DrainForUpdate_FullMethodName {
						drains.Add(1)
					}
					return next(srv, stream)
				}))
				proto.RegisterAgentServer(s, &isolationFake{disabled: true})
				go s.Serve(l)
				t.Cleanup(s.Stop)
				return s, l.Addr().String()
			}
			s, addr := start("127.0.0.1:0")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c, err := DialExistingPinned(ctx, addr)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			s.Stop()
			start(addr)
			rpcCtx, stop := context.WithTimeout(ctx, 1600*time.Millisecond)
			defer stop()
			stream, err := proto.NewAgentClient(c.readConn()).DrainForUpdate(rpcCtx, &proto.DrainForUpdateRequest{OperationToken: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, grpc.WaitForReady(true))
			if err == nil {
				_, err = stream.Recv()
			}
			if drains.Load() != 0 {
				t.Fatalf("replacement received drain without reinspection: %d", drains.Load())
			}
			if err == nil {
				t.Fatal("old transport remained usable")
			}
		})
	}
}
