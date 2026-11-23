package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"cercano/source/server/internal/usage"
	wire "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// AccountingConfigurer is an optional startup interface on worker turn runners.
// It reuses the existing connection; configuring it performs no storage/network I/O.
type AccountingConfigurer interface {
	SetAccountingReceiver(AccountingReceiver) error
}

const workerAccountingDrainBudget = 4 * time.Second // producer/collector 2s + final status 1s + transport slack

type hostAccountingSession struct {
	conn      *grpc.ClientConn
	sink      AccountingReceiver
	source    string
	ctx       context.Context
	cancel    context.CancelFunc
	drain     chan struct{}
	drainOnce sync.Once
	done      chan struct{}
	err       error // read only after done closes
	gapOnce   sync.Once
	failureMu sync.Mutex
	failure   error
}

// source is the same process-local session identity used on observations. No
// connection target, error, or invented conversation/ownership attribution.
func (s *hostAccountingSession) logDiagnostic(reason string) {
	log.Printf("[accounting] reason=%q worker_connection_id=%q", reason, "worker/"+s.source)
}
func (s *hostAccountingSession) gap(reason string) {
	s.gapOnce.Do(func() { s.sink.MarkAccountingIncomplete(reason); s.logDiagnostic(reason) })
}
func (s *hostAccountingSession) requestDrain() { s.drainOnce.Do(func() { close(s.drain) }) }
func (s *hostAccountingSession) run() {
	defer close(s.done)
	defer func() {
		if s.err != nil {
			_ = s.conn.Close()
		}
	}()
	warned := false
	retryDelay := 100 * time.Millisecond
	for {
		err := receiveAccountingControlled(s.ctx, wire.NewWorkerClient(s.conn), s.sink, s.source, func() {
			if warned {
				s.logDiagnostic("recovered")
				warned = false
			}
			retryDelay = 100 * time.Millisecond
		}, s.drain)
		if err == nil {
			return
		}
		if errors.Is(err, errAccountingDrainIncomplete) {
			s.err = err
			s.gap("worker accounting drain incomplete")
			return
		}
		if s.ctx.Err() != nil || s.conn.GetState() == connectivity.Shutdown {
			s.err = err
			s.gap("worker accounting connection ended without a completed drain")
			return
		}
		if !warned {
			s.logDiagnostic("delivery_unavailable")
			warned = true
		}
		timer := time.NewTimer(retryDelay)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			s.err = s.ctx.Err()
			s.gap("worker accounting reconnect interrupted")
			return
		case <-timer.C:
		}
		if retryDelay < 2*time.Second {
			retryDelay *= 2
			if retryDelay > 2*time.Second {
				retryDelay = 2 * time.Second
			}
		}
	}
}
func (s *hostAccountingSession) close(ctx context.Context) (err error) {
	defer func() {
		// Concurrent close callers must not replace an already recorded timeout
		// with a late successful receipt. Uncertainty is terminal for this session.
		s.failureMu.Lock()
		if s.failure == nil && err != nil {
			s.failure = err
		}
		if s.failure != nil {
			err = s.failure
		}
		s.failureMu.Unlock()
		if err != nil {
			s.cancel()
			_ = s.conn.Close()
		}
	}()
	s.requestDrain()
	s.failureMu.Lock()
	prior := s.failure
	s.failureMu.Unlock()
	if prior != nil {
		return prior
	}
	select {
	case <-s.done:
		return s.err
	case <-ctx.Done():
		select {
		case <-s.done:
			return s.err
		default:
		}
		s.failureMu.Lock()
		s.failure = ctx.Err()
		s.failureMu.Unlock()
		go s.gap("worker accounting shutdown exceeded its budget")
		s.cancel()
		return ctx.Err()
	}
}

// One receiver per existing worker connection, never a goroutine per attempt.
// Entries are removed when their stream/process closes; the ordinary turn path
// only looks up/registers a connection and never waits for its accounting RPC.
type accountingState uint8

const (
	accountingActive accountingState = iota
	accountingDraining
	accountingTerminal
)

type accountingConnections struct {
	states       map[*grpc.ClientConn]accountingState
	mu           sync.Mutex
	sink         AccountingReceiver
	sessions     map[*grpc.ClientConn]*hostAccountingSession
	closed       bool
	injectedConn *grpc.ClientConn // process-scoped transport ownership for test dialers
}

func newAccountingConnections(sink AccountingReceiver) *accountingConnections {
	return &accountingConnections{states: make(map[*grpc.ClientConn]accountingState), sink: sink, sessions: make(map[*grpc.ClientConn]*hostAccountingSession)}
}
func (m *accountingConnections) ensure(conn *grpc.ClientConn) error {
	if conn == nil {
		return fmt.Errorf("worker accounting connection is missing")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("worker runner is shutting down")
	}
	if state, ok := m.states[conn]; ok && state != accountingActive {
		return fmt.Errorf("worker accounting connection is draining or terminal")
	}
	if existing, ok := m.sessions[conn]; ok {
		select {
		case <-existing.done:
			m.states[conn] = accountingTerminal
			return fmt.Errorf("worker accounting session is terminal")
		default:
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &hostAccountingSession{conn: conn, sink: m.sink, source: usage.NewIdentity(), ctx: ctx, cancel: cancel, drain: make(chan struct{}), done: make(chan struct{})}
	m.sessions[conn] = s
	m.states[conn] = accountingActive
	go func() {
		s.run()
		s.cancel()
		m.mu.Lock()
		if m.sessions[conn] == s {
			m.states[conn] = accountingTerminal
		}
		m.mu.Unlock()
	}()
	return nil
}
func (m *accountingConnections) drainConnection(ctx context.Context, conn *grpc.ClientConn) error {
	m.mu.Lock()
	s := m.sessions[conn]
	if m.states[conn] != accountingTerminal {
		m.states[conn] = accountingDraining
	}
	m.mu.Unlock()
	if s == nil {
		_ = conn.Close()
		return fmt.Errorf("worker accounting session missing; drain unverified")
	}
	return s.close(ctx)
}
func (m *accountingConnections) shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	sessions := make([]*hostAccountingSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	injected := m.injectedConn
	m.injectedConn = nil
	m.mu.Unlock()
	if injected != nil {
		defer injected.Close()
	}
	// Request all drains before waiting, sharing one overall deadline rather than
	// multiplying the shutdown budget by the number of warm workers.
	for _, s := range sessions {
		s.requestDrain()
	}
	var result error
	for _, s := range sessions {
		result = errors.Join(result, s.close(ctx))
	}
	return result
}

// An accounting-enabled injected dialer mirrors production's warm connection
// lifetime. Disabled test runners retain their original per-turn close behavior.
func (m *accountingConnections) dial(ctx context.Context, dial dialFunc) (*grpc.ClientConn, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("worker runner is shutting down")
	}
	existing := m.injectedConn
	if existing != nil && existing.GetState() != connectivity.Shutdown {
		m.mu.Unlock()
		return existing, nil
	}
	m.mu.Unlock()
	conn, err := dial(ctx)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, fmt.Errorf("worker dial returned no connection")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		conn.Close()
		return nil, fmt.Errorf("worker runner is shutting down")
	}
	existing = m.injectedConn
	if existing != nil && existing.GetState() != connectivity.Shutdown {
		m.mu.Unlock()
		if conn != existing {
			conn.Close()
		}
		return existing, nil
	}
	m.injectedConn = conn
	m.mu.Unlock()
	return conn, nil
}

func (w *workerRunner) SetAccountingReceiver(sink AccountingReceiver) error {
	if sink == nil {
		return nil
	}
	w.accountingMu.Lock()
	defer w.accountingMu.Unlock()
	if w.accounting != nil {
		return fmt.Errorf("worker accounting already configured")
	}
	manager := newAccountingConnections(sink)
	w.accounting = manager
	if w.pool != nil {
		w.pool.setIdleRetirement(func(ctx context.Context, h *workerHandle) {
			_ = manager.drainConnection(ctx, h.conn)
		})
	}
	return nil
}
func (w *workerRunner) accountingManager() *accountingConnections {
	w.accountingMu.RLock()
	defer w.accountingMu.RUnlock()
	return w.accounting
}
