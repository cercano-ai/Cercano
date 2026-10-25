//go:build !darwin || !cgo

package brewrestart

import (
	"context"
	"fmt"
	"net/netip"
)

func RestartInstalled(ctx context.Context, executable string, endpoint netip.AddrPort) (bool, error) {
	return false, fmt.Errorf("Homebrew upgrade restart requires macOS with cgo")
}

func RestartInstalledReady(ctx context.Context, executable string, endpoint netip.AddrPort, probe func(context.Context, int) bool) (bool, error) {
	return RestartInstalled(ctx, executable, endpoint)
}

func RestartInstalledChecked(ctx context.Context, executable string, endpoint netip.AddrPort, probe func(context.Context, int) bool, check ReplacementCheck) (bool, error) {
	return RestartInstalled(ctx, executable, endpoint)
}
