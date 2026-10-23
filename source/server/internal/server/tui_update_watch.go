package server

import (
	"context"
	"os"
	"time"

	"cercano/source/server/internal/updatecoord/enrollment"
)

// WatchEnrolledTUIUpdates runs inside the existing agent lifetime, never as a
// service/process. It exits immediately for an unenrolled launcher. Once bound,
// it observes completed installs without requiring a helper-to-agent command RPC.
// Diagnostics are deduplicated; no failure here should stop the agent.
func (s *Server) WatchEnrolledTUIUpdates(ctx context.Context, launcher, stateRoot string, diagnostic func(error)) {
	s.watchEnrolledTUIUpdates(ctx, launcher, stateRoot, 5*time.Second, diagnostic)
}

// WatchBoundEnrolledTUIUpdates runs with explicit identity binding. It takes an
// expected enrollment record and verifies that the discovered enrollment matches
// exactly before opening any state or emitting notices. This prevents cross-install
// identity confusion.
//
// Observation is read-only and optional; it never registers or polls force control.
func (s *Server) WatchBoundEnrolledTUIUpdates(ctx context.Context, launcher, stateRoot string, expected *enrollment.Record, diagnostic func(error)) {
	s.watchBoundEnrolledTUIUpdates(ctx, launcher, stateRoot, expected, 5*time.Second, diagnostic)
}
func (s *Server) watchEnrolledTUIUpdates(ctx context.Context, launcher, stateRoot string, interval time.Duration, diagnostic func(error)) {
	if ctx == nil || interval <= 0 {
		return
	}
	enrolled, e := enrollment.DiscoverStableLauncher(ctx, launcher)
	if e != nil {
		if !os.IsNotExist(e) && diagnostic != nil {
			diagnostic(e)
		}
		return
	}
	s.watchBoundEnrolledTUIUpdates(ctx, launcher, stateRoot, &enrolled, interval, diagnostic)
}

func (s *Server) watchBoundEnrolledTUIUpdates(ctx context.Context, launcher, stateRoot string, expected *enrollment.Record, interval time.Duration, diagnostic func(error)) {
	if ctx == nil || interval <= 0 || expected == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastError := ""
	for {
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)

		// Re-discover enrollment and verify it matches the fixed expected
		// record on every probe. A mismatch is a different installation:
		// exit without touching its state or emitting anything for it.
		current, e := enrollment.DiscoverStableLauncher(probe, launcher)
		if e != nil {
			if !os.IsNotExist(e) && diagnostic != nil && e.Error() != lastError {
				diagnostic(e)
			}
			lastError = e.Error()
		} else if current != *expected {
			// Enrollment changed: this is a different installation, exit
			cancel()
			return
		} else {
			lastError = ""
		}

		// Only observe installed-completion metadata; retired local control is
		// deliberately not part of the ordinary agent lifetime.
		if e == nil {
			// Use bound sync helper that verifies record match before opening state
			_, e = s.SyncBoundEnrolledTUIUpdate(probe, launcher, stateRoot, expected)
		}

		cancel()
		if ctx.Err() != nil {
			return
		}
		if e != nil {
			if diagnostic != nil && e.Error() != lastError {
				diagnostic(e)
			}
			lastError = e.Error()
		} else {
			lastError = ""
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
