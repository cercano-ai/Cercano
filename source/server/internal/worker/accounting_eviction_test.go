package worker

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	wire "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestAccountingEvictionVersusExplicitDrain(t *testing.T) {
	for _, mode := range []string{"unhealthy-release", "drain-first", "healthy-idle-retirement"} {
		t.Run(mode, func(t *testing.T) {
			ws := New()
			lis := bufconn.Listen(1 << 20)
			ready := make(chan struct{})
			var once sync.Once
			server := grpc.NewServer(grpc.StreamInterceptor(func(srv interface{}, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				return handler(srv, &accountingHandshakeStream{ServerStream: stream, ready: ready, once: &once})
			}))
			wire.RegisterWorkerServer(server, ws)
			served := make(chan struct{})
			go func() { defer close(served); _ = server.Serve(lis) }()
			defer func() { server.Stop(); _ = lis.Close(); <-served }()
			conn, err := grpc.NewClient("passthrough:///eviction-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			sink := &immediateAccountingReceiver{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := &hostAccountingSession{conn: conn, sink: sink, source: "eviction-test", ctx: ctx, cancel: cancel, drain: make(chan struct{}), done: make(chan struct{})}
			go s.run()
			defer func() {
				cancel()
				_ = conn.Close()
				select {
				case <-s.done:
				case <-time.After(5 * time.Second):
					t.Error("accounting goroutine failed to join")
				}
			}()
			// Observe the real Accounting handshake, not a polling delay.
			select {
			case <-ready:
			case <-time.After(5 * time.Second):
				t.Fatal("no accounting handshake")
			}
			h := &workerHandle{conn: conn}
			p := newWorkerPool(nil)
			p.byConv["eviction-test"] = &pooledEntry{handle: h, inUse: true}
			if mode == "drain-first" {
				c, done := context.WithTimeout(context.Background(), 4*time.Second)
				defer done()
				if err := s.close(c); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "healthy-idle-retirement" {
				now := time.Unix(100, 0)
				p.now = func() time.Time { return now }
				p.setIdleRetirement(func(*workerHandle) {
					c, cancel := context.WithTimeout(t.Context(), workerAccountingDrainBudget)
					defer cancel()
					if err := s.close(c); err != nil {
						t.Errorf("idle drain: %v", err)
					}
				})
				p.Release("eviction-test", h, true)
				now = now.Add(time.Minute)
				p.reapIdle(time.Second)
			} else {
				p.Release("eviction-test", h, false)
			}
			select {
			case <-s.done:
			case <-time.After(4 * time.Second):
				t.Fatal("session stuck")
			}
			sink.mu.Lock()
			incomplete := sink.incomplete
			sink.mu.Unlock()
			if incomplete != (mode == "unhealthy-release") {
				t.Fatalf("incomplete=%v error=%v", incomplete, s.err)
			}
			t.Logf("zero writes: incomplete=%v error=%v", incomplete, s.err)
		})
	}
}

// The first server response is the empty ready handshake after writer.attach.
type accountingHandshakeStream struct {
	grpc.ServerStream
	ready chan struct{}
	once  *sync.Once
}

func (s *accountingHandshakeStream) SendMsg(m interface{}) error {
	err := s.ServerStream.SendMsg(m)
	if err == nil {
		s.once.Do(func() { close(s.ready) })
	}
	return err
}
