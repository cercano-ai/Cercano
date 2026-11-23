package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	wire "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func TestRetirementKillExactlyOnce(t *testing.T) {
	var calls atomic.Int32
	h := &workerHandle{onKill: func() { calls.Add(1) }}
	h.Kill()
	h.Kill()
	if calls.Load() != 1 {
		t.Fatalf("cleanup calls=%d", calls.Load())
	}
}

func TestRetirementShutdownIncludesDisplaced(t *testing.T) {
	var calls atomic.Int32
	p := newWorkerPool(func(context.Context, string, uint64) (*workerHandle, error) {
		return &workerHandle{onKill: func() { calls.Add(1) }}, nil
	})
	old, _ := p.Acquire(context.Background(), "same", 1)
	replacement, _ := p.Acquire(context.Background(), "same", 2)
	p.Shutdown()
	if calls.Load() != 2 {
		t.Fatalf("shutdown cleaned %d handles, want both", calls.Load())
	}
	p.Release("same", old, false)
	p.Release("same", replacement, false)
	if calls.Load() != 2 {
		t.Fatal("release repeated cleanup")
	}
}

func TestRetirementUnsafeWinsCancellation(t *testing.T) {
	started, killed, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	r := &turnRetirement{done: make(chan struct{}), detach: func() {}, kill: func() { once.Do(func() { close(killed) }) }, drain: func(context.Context) error { close(started); <-release; return nil }}
	r.retire(ordinaryRetire)
	<-started
	r.retire(unsafeRetire)
	select {
	case <-killed:
	case <-time.After(time.Second):
		t.Fatal("unsafe escalation waited for drain")
	}
	r.retire(ordinaryRetire)
	r.finish(false, func() { t.Error("unsafe handle reused") })
	close(release)
	if r.disposition != unsafeRetire {
		t.Fatal("unsafe disposition lost")
	}
}

func TestRetirementTerminalAccountingCannotResurrect(t *testing.T) {
	conn, err := grpc.NewClient("passthrough:///unused", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	m := newAccountingConnections(&immediateAccountingReceiver{})
	if err := m.drainConnection(t.Context(), conn); err == nil {
		t.Fatal("missing session claimed verified drain")
	}
	if err := m.ensure(conn); err == nil {
		t.Fatal("terminal connection resurrected")
	}
	if conn.GetState() != connectivity.Shutdown {
		t.Fatal("exact connection not destroyed")
	}
}

func TestRetirementAccountingTimeoutStaysUncertain(t *testing.T) {
	conn, err := grpc.NewClient("passthrough:///unused", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	sink := &immediateAccountingReceiver{}
	s := &hostAccountingSession{conn: conn, sink: sink, cancel: func() {}, drain: make(chan struct{}), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if s.close(ctx) == nil {
		t.Fatal("timeout claimed success")
	}
	close(s.done) // a late success cannot erase uncertainty
	if s.close(t.Context()) == nil {
		t.Fatal("late success erased uncertainty")
	}
	if conn.GetState() != connectivity.Shutdown {
		t.Fatal("timed out transport retained")
	}
}

func TestRetirementIdleBatchSharesDeadline(t *testing.T) {
	p := newWorkerPool(nil)
	release := make(chan struct{})
	defer close(release)
	p.setIdleRetirement(func(context.Context, *workerHandle) { <-release })
	var killed atomic.Int32
	for _, id := range []string{"a", "b", "c"} {
		h := &workerHandle{onKill: func() { killed.Add(1) }}
		p.byConv[id] = &pooledEntry{handle: h, lastUsed: time.Now().Add(-time.Hour)}
		p.all[h] = struct{}{}
	}
	start := time.Now()
	p.reapIdle(time.Minute)
	if elapsed := time.Since(start); elapsed > workerAccountingDrainBudget+time.Second {
		t.Fatalf("serial budgets: %s", elapsed)
	}
	if killed.Load() != 3 {
		t.Fatalf("killed=%d", killed.Load())
	}
}

// The child is this test binary, never a live worker or host.
func TestRetirementReaperChild(t *testing.T) {
	if os.Getenv("CERCANO_RETIREMENT_CHILD") != "1" {
		return
	}
	fmt.Print("ready")
	time.Sleep(time.Minute)
	os.Exit(0)
}

type retirementBlockedWriter struct {
	started, release chan struct{}
	once             sync.Once
}

func (w *retirementBlockedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return len(p), nil
}
func TestRetirementBlockedReaperIsObservable(t *testing.T) {
	writer := &retirementBlockedWriter{started: make(chan struct{}), release: make(chan struct{})}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRetirementReaperChild$")
	cmd.Env = append(os.Environ(), "CERCANO_RETIREMENT_CHILD=1")
	cmd.Stdout = writer
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	h := &workerHandle{cmd: cmd, onKill: func() { calls.Add(1) }}
	defer func() { close(writer.release); h.Kill(); <-h.reapDone }()
	select {
	case <-writer.started:
	case <-time.After(5 * time.Second):
		t.Fatal("child did not write")
	}
	done := make(chan struct{})
	go func() { h.Kill(); h.Kill(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Kill blocked on Wait")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := h.waitReaped(ctx); err != context.DeadlineExceeded {
		t.Fatalf("signal falsely proved reaping: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cleanup calls=%d", calls.Load())
	}
}

// Exercise the real producer fence and accounting receipt RPC, rather than
// interpreting connectivity or the absence of observations as drain success.
func TestRetirementAccountingProducerFence(t *testing.T) {
	for _, cooperative := range []bool{true, false} {
		t.Run(fmt.Sprint(cooperative), func(t *testing.T) {
			worker, conn := accountingTestTransport(t)
			sink := &immediateAccountingReceiver{}
			manager := newAccountingConnections(sink)
			if err := manager.ensure(conn); err != nil {
				t.Fatal(err)
			}
			producerCtx, release, err := worker.beginAccountingTurn(t.Context(), &wire.AccountingWork{}, "conversation")
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			joined := make(chan struct{})
			if cooperative {
				go func() { <-producerCtx.Done(); release(); close(joined) }()
			}
			ctx, cancel := context.WithTimeout(t.Context(), workerAccountingDrainBudget)
			defer cancel()
			err = manager.drainConnection(ctx, conn)
			if cooperative {
				if err != nil {
					t.Fatal(err)
				}
				<-joined
			} else {
				if err == nil {
					t.Fatal("noncooperative producer reported success")
				}
				if conn.GetState() != connectivity.Shutdown {
					t.Fatal("failed drain retained exact transport")
				}
				if manager.drainConnection(t.Context(), conn) == nil {
					t.Fatal("terminal uncertainty erased")
				}
			}
			if manager.ensure(conn) == nil {
				t.Fatal("terminal drain resurrected session")
			}
		})
	}
}

func TestRetirementAccountingRegistryBounded(t *testing.T) {
	for _, mode := range []string{"drained", "failed", "unsafe", "shutdown"} {
		t.Run(mode, func(t *testing.T) {
			m := newAccountingConnections(&immediateAccountingReceiver{})
			for i := 0; i < 12; i++ {
				_, conn := accountingTestTransport(t)
				if err := m.ensure(conn); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "drained":
					if err := m.drainConnection(t.Context(), conn); err != nil {
						t.Fatal(err)
					}
				case "failed":
					ctx, cancel := context.WithCancel(t.Context())
					cancel()
					_ = m.drainConnection(ctx, conn)
				case "unsafe":
					_ = conn.Close()
				case "shutdown":
					if err := m.shutdown(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
				_ = conn.Close()
				deadline := time.Now().Add(time.Second)
				for {
					m.mu.Lock()
					n, states := len(m.sessions), len(m.states)
					m.mu.Unlock()
					if n == 0 && states == 0 {
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("terminal registry retained sessions=%d states=%d", n, states)
					}
					time.Sleep(time.Millisecond)
				}
				if m.ensure(conn) == nil {
					t.Fatal("closed connection resurrected")
				}
				if mode == "shutdown" {
					m = newAccountingConnections(&immediateAccountingReceiver{})
				}
			}
		})
	}
}

func TestRetirementAccountingEnsureTerminalRace(t *testing.T) {
	for _, mode := range []string{"drain", "close", "missing"} {
		t.Run(mode, func(t *testing.T) {
			for i := 0; i < 20; i++ {
				_, conn := accountingTestTransport(t)
				m := newAccountingConnections(&immediateAccountingReceiver{})
				if mode != "missing" {
					if err := m.ensure(conn); err != nil {
						t.Fatal(err)
					}
				}
				start := make(chan struct{})
				var wg sync.WaitGroup
				for j := 0; j < 8; j++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						for k := 0; k < 20; k++ {
							_ = m.ensure(conn)
						}
					}()
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					if mode == "close" {
						_ = conn.Close()
					} else {
						_ = m.drainConnection(t.Context(), conn)
					}
				}()
				close(start)
				wg.Wait()
				if m.ensure(conn) == nil {
					t.Fatal("terminal connection resurrected")
				}
				if _, err := m.dial(t.Context(), func(context.Context) (*grpc.ClientConn, error) { return conn, nil }); err == nil {
					t.Fatal("closed injected transport accepted")
				}
				_, fresh := accountingTestTransport(t)
				got, err := m.dial(t.Context(), func(context.Context) (*grpc.ClientConn, error) { return fresh, nil })
				if err != nil || got != fresh {
					t.Fatalf("fresh dial: %v", err)
				}
				if err := m.ensure(got); err != nil {
					t.Fatal(err)
				}
				if err := m.drainConnection(t.Context(), got); err != nil {
					t.Fatal(err)
				}
				m.mu.Lock()
				injected := m.injectedConn
				m.mu.Unlock()
				if injected != nil {
					t.Fatal("terminal injected connection retained")
				}
			}
		})
	}
}
