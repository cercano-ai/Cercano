//go:build !darwin || !cgo

package brewrestart

import (
	"context"
	"fmt"
	"net/netip"
)

func PrepareLegacyHost(context.Context, string, netip.AddrPort) (Identity, error) {
	return Identity{}, fmt.Errorf("verified legacy stop requires macOS with cgo")
}
func StopLegacyHost(context.Context, string, netip.AddrPort, Identity, func() error) error {
	return fmt.Errorf("verified legacy stop requires macOS with cgo")
}
