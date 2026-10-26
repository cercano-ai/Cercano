package agentclient

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// All endpoints are ephemeral loopback fixtures; never launch a real agent.
func TestTransportReplacementIsolation(t *testing.T) {
	for _, setting := range []string{"1", "0", "absent"} {
		for _, replacement := range []string{"enabled", "old", "disabled"} {
			for _, path := range []string{"connect", "DialExisting"} {
				t.Run(setting+"/"+replacement+"/"+path, func(t *testing.T) {
					t.Setenv("CERCANO_DISABLE_CREDENTIALS", setting)
					if setting == "absent" {
						os.Unsetenv("CERCANO_DISABLE_CREDENTIALS")
					}
					var health, mutations atomic.Int32
					start := func(addr string, disabled, old bool) (*grpc.Server, string) {
						l, err := net.Listen("tcp", addr)
						if err != nil {
							t.Fatal(err)
						}
						s := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
							if info.FullMethod == proto.Agent_GetCredentialPolicy_FullMethodName {
								health.Add(1)
								return next(ctx, req)
							}
							mutations.Add(1)
							return &proto.ResetSetupResponse{}, nil
						}))
						if old {
							proto.RegisterAgentServer(s, &proto.UnimplementedAgentServer{})
						} else {
							proto.RegisterAgentServer(s, &isolationFake{disabled: disabled})
						}
						go s.Serve(l)
						t.Cleanup(s.Stop)
						return s, l.Addr().String()
					}
					s, addr := start("127.0.0.1:0", true, false)
					ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
					defer cancel()
					var c *Client
					var err error
					if path == "DialExisting" {
						c, err = DialExisting(ctx, addr)
					} else {
						c, err = connect(ctx, addr, time.Second)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					oldConn := c.readConn()
					s.Stop()
					_, _ = start(addr, replacement == "disabled", replacement == "old")
					health.Store(0)
					rpcCtx, rpcCancel := context.WithTimeout(ctx, 1600*time.Millisecond)
					for {
						err = oldConn.Invoke(rpcCtx, proto.Agent_ResetSetup_FullMethodName, &proto.ResetSetupRequest{}, &proto.ResetSetupResponse{}, grpc.WaitForReady(true))
						// A call racing the initial EOF can fail before gRPC notices
						// the lost transport. Retry only in the normal-policy control.
						if setting == "1" || err == nil || rpcCtx.Err() != nil {
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
					rpcCancel()
					if setting == "absent" || setting == "0" {
						if err != nil || mutations.Load() != 1 {
							t.Fatalf("default reconnect failed: %v, mutations=%d", err, mutations.Load())
						}
						if health.Load() != 0 {
							t.Fatal("default reconnect unexpectedly attested")
						}
						return
					}
					if mutations.Load() != 0 {
						t.Fatalf("BYPASS: old ClientConn sent mutation to %s replacement", replacement)
					}
					if err == nil {
						t.Fatal("old transport unexpectedly usable")
					}
					if health.Load() != 0 {
						t.Fatal("old ClientConn reached replacement")
					}
					c.addr = addr
					c.stopWatch = make(chan struct{})
					c.state = ConnStateReconnecting
					err = c.reconnect(ctx)
					if health.Load() != 1 {
						t.Fatalf("fresh connection missing attestation: %d", health.Load())
					}
					if replacement != "disabled" {
						if !errors.Is(err, ErrCredentialIsolation) {
							t.Fatalf("replacement accepted: %v", err)
						}
					} else {
						if err != nil {
							t.Fatal(err)
						}
						if health.Load() != 1 {
							t.Fatalf("fresh connection missing attestation: %d", health.Load())
						}
						if err := c.readConn().Invoke(ctx, proto.Agent_ResetSetup_FullMethodName, &proto.ResetSetupRequest{}, &proto.ResetSetupResponse{}); err != nil {
							t.Fatal(err)
						}
						if mutations.Load() != 1 {
							t.Fatal("attested connection failed mutation")
						}
					}
				})
			}
		}
	}
}

func TestSingleTransportDialer(t *testing.T) {
	var attempts atomic.Int32
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	dial := singleTransportDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		if attempts.Add(1) == 1 {
			return nil, context.DeadlineExceeded
		}
		return left, nil
	})
	if _, err := dial(context.Background(), "fixture"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := dial(context.Background(), "fixture")
			if err == nil && conn != nil {
				successes.Add(1)
			} else if !errors.Is(err, ErrCredentialIsolation) {
				t.Errorf("unexpected refusal: %v", err)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || attempts.Load() != 2 {
		t.Fatalf("successes=%d attempts=%d", successes.Load(), attempts.Load())
	}
}

func TestIsolatedTransportDialDeadline(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	// Occupy a TCP endpoint without completing the HTTP/2 handshake. The first
	// successful TCP dial consumes the budget even when gRPC never becomes ready.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	done := make(chan struct{})
	var accepted atomic.Int32
	go func() {
		defer close(done)
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			conn.Close()
		}
	}()
	start := time.Now()
	c, err := connect(context.Background(), l.Addr().String(), 1500*time.Millisecond)
	if c != nil {
		c.Close()
		t.Fatal("accepted broken transport")
	}
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("deadline not honored: %v", err)
	}
	l.Close()
	<-done
	if accepted.Load() != 1 {
		t.Fatalf("transport dial spun: %d accepts", accepted.Load())
	}
}
