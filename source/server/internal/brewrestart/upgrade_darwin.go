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
	if _, ok := ctx.Deadline(); !ok {
		return fmt.Errorf("upgrade shutdown requires deadline")
	}
	if !endpoint.IsValid() || !endpoint.Addr().IsLoopback() || endpoint.Port() == 0 {
		return fmt.Errorf("upgrade requires explicit loopback listener")
	}
	identities := map[int]Identity{}
	var host *Identity
	for _, pid := range pids {
		if pid == os.Getpid() {
			continue
		}
		id, err := Inspect(pid)
		if errors.Is(err, syscall.ESRCH) {
			continue
		}
		if err != nil {
			return err
		}
		if id.UID != uint32(os.Getuid()) {
			continue
		}
		identities[pid] = id
		name := filepath.Base(id.Executable)
		owns, err := HoldsListener(id, endpoint)
		if err != nil {
			return fmt.Errorf("upgrade blocker pid=%d: %w", pid, err)
		}
		if owns {
			if name != "cercano" && name != "cercano-agent" {
				return fmt.Errorf("upgrade blocker pid=%d executable=%s: custom listener identity is unsupported", pid, id.Executable)
			}
			if host != nil {
				return fmt.Errorf("multiple upgrade listener owners")
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
			return err
		}
		if role == "model-host" {
			continue
		}
		if host == nil || id.ParentPID != host.PID || id.Executable != host.Executable || role != "worker" {
			return fmt.Errorf("upgrade blocker pid=%d: no verified host/worker association", pid)
		}
	}
	if host == nil {
		return nil
	}
	// Stop the producer BEFORE the final child scan. Keep it stopped until KILL:
	// resuming even to deliver TERM would reopen the historical spawn race.
	// On refusal/cancellation, resume only this same process, never a reused PID.
	frozen := true
	defer func() {
		if frozen {
			_ = signalIdentity(*host, syscall.SIGCONT)
		}
	}()
	if err := freezeUpgradeHost(ctx, *host); err != nil {
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
			if role == "model-host" {
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
	// remains frozen, so TERM is pending there; workers may exit normally. No
	// negative/group PID is ever signalled (workers have independent groups).
	targets := map[int]Identity{host.PID: *host}
	for pid, id := range workers {
		targets[pid] = id
	}
	for _, id := range targets {
		if err := signalIdentity(id, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
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
