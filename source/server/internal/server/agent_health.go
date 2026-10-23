// Additive, read-only Agent lifecycle health RPC (spec: cross-platform-updates
// agent health RPC). GetAgentHealth observes — never acts: no listener, no
// control authorization, no compatibility gating, no live actions or push, and
// no work-admission lifetime (the RPC is exempt in update_admission.go). It
// exists so an updater or monitor can honestly answer "is this agent process
// ready to serve and safe to leave running?" before deciding what to do next.
package server

import (
	"context"
	"os"
	"sync"

	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/proto"
)

// Stable machine-readable reasons reported in GetAgentHealthResponse
// not_ready_reasons. Callers should match on these strings, not on prose.
const (
	// HealthReasonStartupIncomplete: the process entrypoint has not yet marked
	// startup complete (MarkStartupComplete). An agent that cannot prove it
	// finished initialization must never report ready.
	HealthReasonStartupIncomplete = "startup_incomplete"
	// HealthReasonAdmissionPaused: update preparation holds the admission
	// pause (updateAdmission.paused — pauseWhenIdle or sealForForce). This is
	// observational; health itself never arms or releases the pause.
	HealthReasonAdmissionPaused = "admission_paused"
	// HealthReasonShutdownBegun: BeginShutdown ran — the process is heading to
	// its normal stop path and must not be treated as usable.
	HealthReasonShutdownBegun = "shutdown_begun"
)

// agentLifecycle records the two externally-owned lifecycle transitions the
// health probe observes. Both are one-way; the zero value is deliberately
// "not ready": a server that never wires startup ownership (tests, embedders)
// stays honestly not-ready rather than defaulting to ready.
type agentLifecycle struct {
	mu              sync.RWMutex
	startupComplete bool
	shutdownBegun   bool
}

func (l *agentLifecycle) markStartupComplete() {
	l.mu.Lock()
	l.startupComplete = true
	l.mu.Unlock()
}

func (l *agentLifecycle) markShutdownBegun() {
	l.mu.Lock()
	l.shutdownBegun = true
	l.mu.Unlock()
}

func (l *agentLifecycle) snapshot() (startupComplete bool, shutdownBegun bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.startupComplete, l.shutdownBegun
}

// MarkStartupComplete is called by the process entrypoint once the listener is
// bound, every service is wired, and the gRPC server is serving. The binary
// owns this call; servers constructed without it (tests, embedders) stay
// not-ready. It is idempotent and one-way.
func (s *Server) MarkStartupComplete() {
	s.lifecycle.markStartupComplete()
}

// admissionPausedSnapshot is an observational accessor: it never mutates the
// admission gate. updateAdmission.paused means update preparation is active
// (pauseWhenIdle) or the process has been sealed for a forced stop.
func admissionPausedSnapshot(g *updateAdmission) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.paused
}

// GetAgentHealth implements proto.AgentAgent — read-only lifecycle probe.
// ready is true only when startup is complete, update-work admission is not
// paused, and shutdown has not begun. version defaults to "dev" when the
// entrypoint never set one (mirrors trajectory_export.go). pid is purely
// observational — the same identity-guard spirit as
// ShutdownAgentWhenIdleRequest.expected_pid, never authentication: PID
// equality proves nothing about caller identity or installation ownership.
func (s *Server) GetAgentHealth(ctx context.Context, req *proto.GetAgentHealthRequest) (*proto.GetAgentHealthResponse, error) {
	version := s.buildVersion
	if version == "" {
		version = "dev"
	}
	startupComplete, shutdownBegun := s.lifecycle.snapshot()
	reasons := make([]string, 0, 3)
	if !startupComplete {
		reasons = append(reasons, HealthReasonStartupIncomplete)
	}
	if admissionPausedSnapshot(&s.updateWork) {
		reasons = append(reasons, HealthReasonAdmissionPaused)
	}
	if shutdownBegun {
		reasons = append(reasons, HealthReasonShutdownBegun)
	}
	return &proto.GetAgentHealthResponse{
		// CheckAllowed also fails closed for malformed settings. Normal startup
		// refuses those via config.Load/ValidateEnvironment before serving.
		// This is an explicit process policy, not a runtime revocation protocol.
		CredentialsDisabled: secrets.CheckAllowed() != nil,
		Version:             version,
		Pid:                 int32(os.Getpid()),
		Ready:               len(reasons) == 0,
		NotReadyReasons:     reasons,
	}, nil
}
