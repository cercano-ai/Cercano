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

// StopUpgradeHost terminates verified DB-writing hosts, including embedded MCP.
// Names alone never authorize signalling. The caller holds the SDK launch lock across this operation and
// migration. Unavailable unrelated images are not evidence of host writers.
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
	targets, err := discoverUpgradeTargets(endpoint, pids, false)
	fmt.Fprintf(os.Stderr, "[cercano] discovery elapsed=%s\n", time.Since(discoveryStarted))
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := stopVerifiedUpgradeHost(ctx, target, afterTERM); err != nil {
			return err
		}
	}
	return nil
}

type upgradeTarget struct {
	Identity
	endpoint netip.AddrPort
}

func stopVerifiedUpgradeHost(ctx context.Context, target upgradeTarget, afterTERM func()) error {
	return stopVerifiedUpgradeHostWithSignal(ctx, target, afterTERM, signalIdentity)
}

// signal is an error-injection seam for disposable tests, never discovery authority.
func stopVerifiedUpgradeHostWithSignal(ctx context.Context, target upgradeTarget, afterTERM func(), signal func(Identity, syscall.Signal) error) (retErr error) {
	// Only independently identified DB-writing hosts are targets. Embedded MCP
	// runs in the host; descendants need not exit and receive no signals here.
	if err := ctx.Err(); err != nil {
		return err
	}
	host := target.Identity
	owns, err := HoldsListener(host, target.endpoint)
	if err != nil || !owns {
		return fmt.Errorf("host lost listener pid=%d: %v", host.PID, err)
	}
	targets := map[int]Identity{host.PID: host}
	// After TERM, finish bounded cleanup on cancellation, but preserve the
	// failure so the caller cannot proceed to backup or migration.
	defer func() {
		if retErr == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for _, id := range targets {
			if err := signal(id, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
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
		if err := signal(id, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
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
		if err := signal(id, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}
	for _, id := range targets {
		if err := waitProcessExit(ctx, id); err != nil {
			return fmt.Errorf("upgrade pid=%d not exited: %w", id.PID, err)
		}
	}
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
	_, err = discoverUpgradeHostInventory(endpoint, pids, true)
	return err
}

func discoverUpgradeHost(endpoint netip.AddrPort, pids []int) (*Identity, error) {
	return discoverUpgradeHostInventory(endpoint, pids, false)
}

// Diagnostic mode collects every refusal using production classification. It never
// returns a host when any blocker exists; shutdown retains first-error refusal.
func discoverUpgradeHostInventory(endpoint netip.AddrPort, pids []int, aggregate bool) (*Identity, error) {
	targets, err := discoverUpgradeTargets(endpoint, pids, aggregate)
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		owns, err := HoldsListener(target.Identity, endpoint)
		if err != nil {
			return nil, err
		}
		if owns {
			id := target.Identity
			return &id, nil
		}
	}
	return nil, nil
}

func discoverUpgradeTargets(endpoint netip.AddrPort, pids []int, aggregate bool) ([]upgradeTarget, error) {
	var blockers []error
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
			if !aggregate {
				return nil, err
			}
			blockers = append(blockers, fmt.Errorf("upgrade blocker pid=%d: %w", pid, err))
			continue
		}
		if id.PID == 0 || id.UID != uint32(os.Getuid()) {
			continue
		}
		identities[pid] = id
		name := filepath.Base(id.Executable)
		owns, err := HoldsListener(id, endpoint)
		if err != nil {
			if !aggregate {
				return nil, fmt.Errorf("upgrade blocker pid=%d: %w", pid, err)
			}
			blockers = append(blockers, fmt.Errorf("upgrade blocker pid=%d: %w", pid, err))
			continue
		}
		if owns {
			if name != "cercano" && name != "cercano-agent" {
				if !aggregate {
					return nil, fmt.Errorf("upgrade blocker pid=%d executable=%s: custom listener identity is unsupported", pid, id.Executable)
				}
				blockers = append(blockers, fmt.Errorf("upgrade blocker pid=%d executable=%s: custom listener identity is unsupported", pid, id.Executable))
				continue
			}
			if host != nil {
				if !aggregate {
					return nil, fmt.Errorf("multiple upgrade listener owners")
				}
				blockers = append(blockers, fmt.Errorf("multiple upgrade listener owners"))
				continue
			}
			copy := id
			host = &copy
		}
	}
	// Identify each additional host independently, before disrupting any host.
	targets := []upgradeTarget{}
	hosts := map[int]Identity{}
	if host != nil {
		targets = append(targets, upgradeTarget{*host, endpoint})
		hosts[host.PID] = *host
	}
	for _, pid := range pids {
		id, exists := identities[pid]
		if !exists || (host != nil && pid == host.PID) {
			continue
		}
		name := filepath.Base(id.Executable)
		if name != "cercano" && name != "cercano-agent" {
			continue
		}
		role, err := processRole(id)
		if err == nil && role != "agent" && role != "development-host" {
			continue
		}
		var ownEndpoint netip.AddrPort
		if err == nil {
			ownEndpoint, err = upgradeListenerEndpoint(id)
		}
		if err != nil {
			blocker := fmt.Errorf("upgrade blocker pid=%d: %w", pid, err)
			if !aggregate {
				return nil, blocker
			}
			blockers = append(blockers, blocker)
			continue
		}
		hosts[pid] = id
		targets = append(targets, upgradeTarget{id, ownEndpoint})
	}
	// Preflight known agents before disrupting anything. A shared model host is
	// never a conversation writer and is never signalled, even if it is a child.
	for _, pid := range pids {
		id, exists := identities[pid]
		if !exists {
			continue
		}
		name := filepath.Base(id.Executable)
		if name != "cercano" && name != "cercano-agent" {
			continue
		}
		if _, isHost := hosts[pid]; isHost {
			continue
		}
		role, err := processRole(id)
		if err != nil {
			if !aggregate {
				return nil, err
			}
			blockers = append(blockers, fmt.Errorf("upgrade blocker pid=%d: %w", pid, err))
			continue
		}
		// These roles persist through hosts, not directly to the database.
		// Exclusion grants no signal authority.
		if role == "model-host" || role == "worker" {
			continue
		}
		blocker := fmt.Errorf("upgrade blocker pid=%d: unresolved possible writer role %q", pid, role)
		if !aggregate {
			return nil, blocker
		}
		blockers = append(blockers, blocker)
	}
	if len(blockers) > 0 {
		return nil, errors.Join(blockers...)
	}
	return targets, nil
}

func safeUpgradeRole(role string) string {
	switch role {
	case "worker", "model-host", "agent", "development-host":
		return role
	case "":
		return "missing"
	default:
		return "unrecognized"
	}
}
