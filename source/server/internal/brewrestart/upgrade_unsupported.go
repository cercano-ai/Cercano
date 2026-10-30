//go:build !darwin || !cgo

package brewrestart

import (
	"context"
	"fmt"
	"net/netip"
)

func StopUpgradeHost(context.Context, netip.AddrPort) error {
	return fmt.Errorf("verified upgrade shutdown requires macOS with cgo; no migration attempted")
}

func IsSharedModelHost(pid int) bool { return false }

func PreflightUpgradeHost(endpoint netip.AddrPort) error {
	return fmt.Errorf("upgrade discovery requires macOS with cgo")
}
