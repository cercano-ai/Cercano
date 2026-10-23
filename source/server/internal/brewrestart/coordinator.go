package brewrestart

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"
)

// restartOps separates lifecycle effects so sequencing and refusal paths can
// be tested without touching a live agent. Production adapters use kernel
// inspection, the shared CLI launch lock and the existing ShutdownAgent RPC.
type restartOps struct {
	source    processSource
	capture   func(Identity) (LaunchState, error)
	preflight func(string, LaunchState) error
	lock      func(context.Context, LaunchState) (func(), error)
	shutdown  func(context.Context, Identity, netip.AddrPort) error
	waitExit  func(context.Context, Identity) error
	start     func(string, LaunchState) (Identity, error)
	ready     func(context.Context, Identity, netip.AddrPort) error
}

// errReplacementExited reports that the replacement process exited, or its
// PID now names another process, before it became ready.
var errReplacementExited = errors.New("replacement exited during startup")

// readyPoll is how often withReady re-checks a starting replacement.
const readyPoll = 100 * time.Millisecond

// withReady replaces ops' readiness check: instead of watching the old
// endpoint, it polls probe with the replacement's process ID until it reports
// ready, failing at once when alive reports that the replacement (by its full
// identity, so a reused PID does not count) is gone, and at ctx's deadline
// otherwise.
func withReady(ops restartOps, alive func(Identity) error, probe func(ctx context.Context, pid int) bool) restartOps {
	ops.ready = func(ctx context.Context, id Identity, _ netip.AddrPort) error {
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := alive(id); err != nil {
				return err
			}
			if probe(ctx, id.PID) {
				return nil
			}
			timer := time.NewTimer(readyPoll)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return ops
}

func coordinateRestart(ctx context.Context, ops restartOps, newExecutable string, uid uint32, endpoint netip.AddrPort) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	owner, err := discover(ops.source, newExecutable, uid, endpoint)
	if err != nil || owner == nil {
		return false, err
	}
	state, err := ops.capture(*owner)
	if err != nil {
		return false, err
	}
	if !state.Identity.SameProcess(*owner) {
		return false, fmt.Errorf("captured launch state belongs to a different process")
	}
	// All local validation must succeed before the destructive boundary.
	if err := ops.preflight(newExecutable, state); err != nil {
		return false, err
	}
	release, err := ops.lock(ctx, state)
	if err != nil {
		return false, err
	}
	defer release()
	// A reconnecting client or another upgrade may have won the launch lock.
	// Never apply a stale snapshot to whichever process happens to listen now.
	current, err := discover(ops.source, newExecutable, uid, endpoint)
	if err != nil {
		return false, err
	}
	if current == nil || !owner.SameProcess(*current) {
		return false, fmt.Errorf("agent changed while waiting for launch lock; retry the restart")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := ops.shutdown(ctx, *owner, endpoint); err != nil {
		return false, fmt.Errorf("shutdown request failed; no replacement started: %w", err)
	}
	if err := ops.waitExit(ctx, *owner); err != nil {
		return false, fmt.Errorf("old agent has not confirmed exit; no replacement started: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("agent stopped but restart deadline expired: %w", err)
	}
	replacement, err := ops.start(newExecutable, state)
	if err != nil {
		return false, fmt.Errorf("agent stopped but replacement could not start; start cercano manually: %w", err)
	}
	if err := ops.ready(ctx, replacement, endpoint); err != nil {
		return false, fmt.Errorf("replacement started but readiness was not confirmed; check agent logs before retrying: %w", err)
	}
	return true, nil
}
