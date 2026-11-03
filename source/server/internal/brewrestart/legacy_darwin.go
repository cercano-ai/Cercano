//go:build darwin && cgo

package brewrestart

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
)

// PrepareLegacyHost identifies only the exact recorded source listener. It does
// not use upgrade-wide discovery (which intentionally includes other writers).
func PrepareLegacyHost(ctx context.Context, host string, endpoint netip.AddrPort) (Identity, error) {
	if host == "" || !endpoint.IsValid() || !endpoint.Addr().IsLoopback() || endpoint.Port() == 0 {
		return Identity{}, fmt.Errorf("exact source loopback listener required")
	}
	pids, err := ListCandidates()
	if err != nil {
		return Identity{}, err
	}
	var found Identity
	for _, pid := range pids {
		if err := ctx.Err(); err != nil {
			return Identity{}, err
		}
		id, err := Inspect(pid)
		if err != nil {
			continue
		}
		owns, err := HoldsListener(id, endpoint)
		if err != nil {
			return Identity{}, err
		}
		if !owns {
			continue
		}
		name := filepath.Base(id.Executable)
		if pid == os.Getpid() || id.UID != uint32(os.Getuid()) || id.StartSeconds == 0 || (name != "cercano" && name != "cercano-agent") {
			return Identity{}, fmt.Errorf("unsupported source listener")
		}
		if err := verifyLegacyHostArg(id, host); err != nil {
			return Identity{}, err
		}
		if found.PID != 0 {
			return Identity{}, fmt.Errorf("ambiguous source listener")
		}
		found = id
	}
	if found.PID == 0 {
		return Identity{}, fmt.Errorf("source listener not found")
	}
	return found, nil
}

// StopLegacyHost reuses the launcher's single-target TERM/wait/KILL/verified-exit
// implementation. The caller holds source launch exclusion throughout.
func StopLegacyHost(ctx context.Context, host string, endpoint netip.AddrPort, expected Identity, validate func() error) error {
	if _, ok := ctx.Deadline(); !ok {
		return fmt.Errorf("legacy shutdown requires deadline")
	}
	now, err := PrepareLegacyHost(ctx, host, endpoint)
	if err != nil {
		return err
	}
	if !now.SameProcess(expected) {
		return fmt.Errorf("source process identity changed")
	}
	// Before entering cleanup authority, revalidate durable approval and epoch.
	return stopVerifiedUpgradeHostValidated(ctx, upgradeTarget{expected, endpoint}, nil, signalIdentity, validate)
}
