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
)

// StopUpgradeHost uses the ordinary shutdown control, never signals by PID or
// process name. The caller holds the SDK launch lock across this operation and
// migration. Unknown candidates remain blockers, not exceptions to detection.
func StopUpgradeHost(ctx context.Context, endpoint netip.AddrPort) error {
	pids, err := ListCandidates()
	if err != nil {
		return err
	}
	return stopUpgradeHost(ctx, endpoint, pids)
}

// The PID boundary permits disposable-process tests without inspecting the
// developer's agents. All identity, socket, RPC and exit checks remain native.
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
	associated := map[int]Identity{}
	if host != nil {
		associated[host.PID] = *host
		for changed := true; changed; {
			changed = false
			for pid, id := range identities {
				if _, ok := associated[pid]; ok {
					continue
				}
				parent, child := associated[id.ParentPID]
				if child && (id.StartSeconds < parent.StartSeconds || (id.StartSeconds == parent.StartSeconds && id.StartMicroseconds < parent.StartMicroseconds)) {
					return fmt.Errorf("upgrade blocker pid=%d: invalid parent start time", pid)
				}
				if child || (host.ProcessGroup == host.PID && id.ProcessGroup == host.PID && id.StartSeconds >= host.StartSeconds) {
					associated[pid] = id
					changed = true
				}
			}
		}
	}
	// Preflight every known candidate before disrupting even the supported host.
	for pid, id := range identities {
		name := filepath.Base(id.Executable)
		if name == "cercano" || name == "cercano-agent" {
			if _, ok := associated[pid]; !ok {
				return fmt.Errorf("upgrade blocker pid=%d executable=%s: no verified host/worker association", pid, id.Executable)
			}
		}
	}
	if host == nil {
		return nil
	}
	// Revalidate captured relationships before shutdown. Reparenting is not proof
	// of ownership; exit observation below uses immutable start time instead.
	for pid, id := range associated {
		now, err := Inspect(pid)
		if err != nil || !now.SameProcess(id) || now.ParentPID != id.ParentPID || now.ProcessGroup != id.ProcessGroup {
			return fmt.Errorf("upgrade blocker pid=%d: process relationship changed", pid)
		}
	}
	if err := requestShutdown(ctx, *host, endpoint); err != nil {
		return fmt.Errorf("upgrade blocker host pid=%d: %w", host.PID, err)
	}
	for pid, id := range associated {
		if err := waitProcessExit(ctx, id); err != nil {
			return fmt.Errorf("upgrade blocker pid=%d executable=%s has not exited: %w", pid, id.Executable, err)
		}
	}
	return nil
}
