//go:build darwin && cgo

package brewrestart

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// StopUpgradeHost externally terminates a verified listener and its workers.
// Names alone never authorize signalling. The caller holds the SDK launch lock across this operation and
// migration. Unknown candidates remain blockers, not exceptions to detection.
func StopUpgradeHost(ctx context.Context, endpoint netip.AddrPort) error {
	pids, err := ListCandidates()
	if err != nil {
		return err
	}
	return stopUpgradeHost(ctx, endpoint, pids)
}

// The PID boundary permits disposable-process tests without inspecting the
// developer's agents. All identity, socket, signal and exit checks remain native.
func stopUpgradeHost(ctx context.Context, endpoint netip.AddrPort, pids []int) error {
	return stopUpgradeHostObserved(ctx, endpoint, pids, nil)
}

// afterTERM is a package-test observation seam, never an alternative signal path.
func stopUpgradeHostObserved(ctx context.Context, endpoint netip.AddrPort, pids []int, afterTERM func()) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return fmt.Errorf("upgrade shutdown requires deadline")
	}
	if !endpoint.IsValid() || !endpoint.Addr().IsLoopback() || endpoint.Port() == 0 {
		return fmt.Errorf("upgrade requires explicit loopback listener")
	}
	discoveryStarted := time.Now()
	host, err := discoverUpgradeHost(endpoint, pids)
	fmt.Fprintf(os.Stderr, "[cercano] discovery elapsed=%s\n", time.Since(discoveryStarted))
	if err != nil {
		return err
	}
	if host == nil {
		return nil
	}
	// Stop the producer BEFORE the final child scan. Keep it stopped until KILL:
	// resuming even to deliver TERM would reopen the historical spawn race.
	// On refusal/cancellation, resume only this same process, never a reused PID.
	frozen := true
	frozenChildren := map[int]stoppedIdentity{}
	var hostStop stoppedIdentity
	defer func() {
		if frozen {
			for _, id := range frozenChildren {
				retErr = errors.Join(retErr, resumeUpgradeProcess(id))
			}
			retErr = errors.Join(retErr, resumeUpgradeProcess(hostStop))
		}
	}()
	if err := freezeUpgradeHost(ctx, *host, &hostStop); err != nil {
		return err
	}
	owns, err := HoldsListener(*host, endpoint)
	if err != nil || !owns {
		return fmt.Errorf("frozen host lost listener: %v", err)
	}
	workers := map[int]Identity{}
	// A fork already in flight may still be execing. Wait for its role to settle;
	// never classify such a child by name or group alone.
	for {
		candidates, err := upgradeChildren(host.PID)
		if err != nil {
			return err
		}
		pending := false
		for _, pid := range candidates {
			id, err := Inspect(pid)
			if errors.Is(err, syscall.ESRCH) {
				continue
			}
			if err != nil {
				return err
			}
			if id.ParentPID != host.PID {
				continue
			}
			if id.StartSeconds < host.StartSeconds || (id.StartSeconds == host.StartSeconds && id.StartMicroseconds < host.StartMicroseconds) {
				return fmt.Errorf("invalid child identity")
			}
			role, err := processRole(id)
			if err != nil {
				pending = true
				continue
			}
			shared, sharedErr := verifiedModelHost(id)
			if sharedErr != nil {
				return sharedErr
			}
			if shared {
				continue
			}
			if id.Executable == host.Executable && role == "worker" {
				workers[pid] = id
				continue
			}
			// Unknown children are not ours to kill. Refuse rather than leave possible
			// work running and authorize migration.
			pending = true
		}
		if !pending {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("unresolved host child: %w", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	// Historical escalation: TERM, twenty 100ms exit polls, then KILL. The host
	// and worker tree remain frozen, so TERM stays pending until escalation. No
	// negative/group PID is ever signalled (workers have independent groups).
	targets := map[int]Identity{host.PID: *host}
	for pid, id := range workers {
		targets[pid] = id
	}
	// Workers are producers too: RunCommand owns a separate process group.
	// Freeze the verified tree before TERM can orphan its tool descendants.
	if err := freezeUpgradeDescendants(ctx, *host, workers, frozenChildren); err != nil {
		return err
	}
	for pid, id := range frozenChildren {
		targets[pid] = id.Identity
	}
	if err := ctx.Err(); err != nil {
		return err // Still reversible: no TERM sent, deferred CONT is safe.
	}
	// Once TERM may have been delivered, resuming the producer is not rollback:
	// it can spawn replacement workers or deliver pending host shutdown while
	// orphaning surviving work. Finish bounded identity-checked cleanup even if
	// the caller cancels. Preserve the caller error so migration stays refused.
	frozen = false
	defer func() {
		if retErr == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, id := range targets {
			if err := signalIdentity(id, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				retErr = errors.Join(retErr, fmt.Errorf("upgrade cleanup pid=%d: %w", id.PID, err))
			}
		}
		for _, id := range targets {
			if err := waitProcessExit(cleanupCtx, id); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("upgrade cleanup pid=%d not exited: %w", id.PID, err))
			}
		}
	}()
	for _, id := range targets {
		if err := signalIdentity(id, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}
	if afterTERM != nil {
		afterTERM()
	}
	for i := 0; i < 20; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		for pid, id := range targets {
			now, err := Inspect(pid)
			if errors.Is(err, syscall.ESRCH) || (err == nil && !now.SameProcess(id)) {
				delete(targets, pid)
			} else if err != nil {
				return err
			}
		}
		if len(targets) == 0 {
			break
		}
	}
	for _, id := range targets {
		if err := signalIdentity(id, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}
	for _, id := range targets {
		if err := waitProcessExit(ctx, id); err != nil {
			return fmt.Errorf("upgrade pid=%d not exited: %w", id.PID, err)
		}
	}
	frozen = false
	return nil
}

// PreflightUpgradeHost executes production enumeration and the same read-only
// discovery used by shutdown. It never opens a database or signals a process.
func PreflightUpgradeHost(endpoint netip.AddrPort) error {
	if !endpoint.IsValid() || !endpoint.Addr().IsLoopback() || endpoint.Port() == 0 || endpoint.Addr().Zone() != "" {
		return fmt.Errorf("upgrade requires explicit loopback listener")
	}
	pids, err := ListCandidates()
	if err != nil {
		return err
	}
	_, err = discoverUpgradeHost(endpoint, pids)
	return err
}

func discoverUpgradeHost(endpoint netip.AddrPort, pids []int) (*Identity, error) {
	identities := map[int]Identity{}
	var host *Identity
	for _, pid := range pids {
		if pid == os.Getpid() {
			continue
		}
		id, err := inspectUpgradeCandidate(pid, endpoint)
		if errors.Is(err, syscall.ESRCH) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if id.PID == 0 || id.UID != uint32(os.Getuid()) {
			continue
		}
		identities[pid] = id
		name := filepath.Base(id.Executable)
		owns, err := HoldsListener(id, endpoint)
		if err != nil {
			return nil, fmt.Errorf("upgrade blocker pid=%d: %w", pid, err)
		}
		if owns {
			if name != "cercano" && name != "cercano-agent" {
				return nil, fmt.Errorf("upgrade blocker pid=%d executable=%s: custom listener identity is unsupported", pid, id.Executable)
			}
			if host != nil {
				return nil, fmt.Errorf("multiple upgrade listener owners")
			}
			copy := id
			host = &copy
		}
	}
	// Preflight known agents before disrupting anything. A shared model host is
	// never a conversation writer and is never signalled, even if it is a child.
	for pid, id := range identities {
		name := filepath.Base(id.Executable)
		if name != "cercano" && name != "cercano-agent" {
			continue
		}
		if host != nil && pid == host.PID {
			continue
		}
		role, err := processRole(id)
		if err != nil {
			return nil, err
		}
		if role == "model-host" {
			continue
		}
		if host == nil || id.ParentPID != host.PID || id.Executable != host.Executable || role != "worker" {
			return nil, fmt.Errorf("upgrade blocker pid=%d: no verified host/worker association", pid)
		}
	}
	return host, nil
}
