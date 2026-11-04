package agentclient

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// Exercise the actual DialExisting watcher, not an explicitly invoked reconnect.
// A replacement must not become usable while its policy RPC is still pending.
func TestDialExistingAutomaticRecoveryCredentialIsolation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	oldLaunch := launchServerProcess
	var launches atomic.Int32
	launchServerProcess = func(string) (string, error) {
		launches.Add(1)
		return "", errors.New("unexpected local launch")
	}
	defer func() { launchServerProcess = oldLaunch }()

	for _, policy := range []string{"disabled", "enabled", "legacy"} {
		t.Run(policy, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			wait := func(what string, ready func() bool) {
				t.Helper()
				for !ready() {
					select {
					case <-ctx.Done():
						t.Fatalf("timed out waiting for %s", what)
					case <-time.After(10 * time.Millisecond):
					}
				}
			}
			var attestations, mutations atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			var enteredOnce, releaseOnce sync.Once
			releasePolicy := func() { releaseOnce.Do(func() { close(release) }) }
			defer releasePolicy()
			start := func(addr string, replacement bool) (string, func()) {
				t.Helper()
				l, err := net.Listen("tcp", addr)
				if err != nil {
					t.Fatal(err)
				}
				s := grpc.NewServer(grpc.UnaryInterceptor(func(rpcCtx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
					if info.FullMethod == proto.Agent_GetCredentialPolicy_FullMethodName {
						attestations.Add(1)
						if replacement {
							enteredOnce.Do(func() { close(entered) })
							select {
							case <-release:
							case <-rpcCtx.Done():
								return nil, rpcCtx.Err()
							}
						}
						return next(rpcCtx, req)
					}
					mutations.Add(1)
					return &proto.ResetSetupResponse{}, nil
				}))
				if replacement && policy == "legacy" {
					proto.RegisterAgentServer(s, &proto.UnimplementedAgentServer{})
				} else {
					proto.RegisterAgentServer(s, &isolationFake{disabled: !replacement || policy == "disabled"})
				}
				go s.Serve(l)
				t.Cleanup(s.Stop)
				return l.Addr().String(), s.Stop
			}

			addr, stop := start("127.0.0.1:0", false)
			c, err := DialExisting(ctx, addr)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			wait("initial watcher state", func() bool { return c.State() == ConnStateConnected })
			if attestations.Load() != 1 || !c.connectOnly || c.AutoLaunched || c.addr != addr {
				t.Fatal("initial endpoint was not attested with connect-only intent")
			}
			oldConn := c.readConn()
			stop()
			wait("automatic recovery", func() bool { return c.State() == ConnStateReconnecting })
			// Leave the endpoint absent through an unavailable redial, including
			// the first attempt at which ordinary clients could launch locally.
			time.Sleep(3500 * time.Millisecond)
			if launches.Load() != 0 {
				t.Fatal("unavailable connect-only endpoint triggered a local launch")
			}
			start(addr, true)
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("watcher did not attest replacement")
			}
			if c.State() == ConnStateConnected || c.readConn() != oldConn {
				t.Fatal("replacement admitted before attestation completed")
			}
			invoke := func(conn *grpcConn) error {
				rpcCtx, rpcCancel := context.WithTimeout(ctx, 250*time.Millisecond)
				defer rpcCancel()
				return conn.Invoke(rpcCtx, proto.Agent_ResetSetup_FullMethodName, &proto.ResetSetupRequest{}, &proto.ResetSetupResponse{}, grpc.WaitForReady(true))
			}
			if err := invoke(oldConn); err == nil || mutations.Load() != 0 {
				t.Fatalf("old transport allowed mutation during attestation: err=%v mutations=%d", err, mutations.Load())
			}
			releasePolicy()
			// The watcher holds reconnectMu during attestation. Wait for that
			// operation to finish without calling reconnect or stopping the watcher.
			finished := make(chan struct{})
			go func() {
				c.reconnectMu.Lock()
				c.reconnectMu.Unlock()
				close(finished)
			}()
			select {
			case <-finished:
			case <-ctx.Done():
				t.Fatal("automatic recovery did not finish admission")
			}
			if attestations.Load() != 2 {
				t.Fatalf("expected initial and fresh attestation, got %d", attestations.Load())
			}
			if policy == "disabled" {
				if c.State() != ConnStateConnected || c.readConn() == oldConn {
					t.Fatal("attested endpoint was not installed by watcher")
				}
				if err := invoke(c.readConn()); err != nil || mutations.Load() != 1 {
					t.Fatalf("attested endpoint unusable: err=%v mutations=%d", err, mutations.Load())
				}
			} else {
				if c.State() == ConnStateConnected || c.readConn() != oldConn {
					t.Fatal("watcher admitted endpoint without disabled-credentials attestation")
				}
				if err := invoke(c.readConn()); err == nil || mutations.Load() != 0 {
					t.Fatalf("rejected endpoint allowed mutation: err=%v mutations=%d", err, mutations.Load())
				}
			}
			before := mutations.Load()
			if err := invoke(oldConn); err == nil || mutations.Load() != before {
				t.Fatal("old transport became usable after admission decision")
			}
			if launches.Load() != 0 || c.AutoLaunched || !c.connectOnly || c.addr != addr {
				t.Fatal("automatic recovery lost connect-only intent")
			}
		})
	}
}
