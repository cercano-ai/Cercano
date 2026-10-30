//go:build darwin && cgo && upgradefixture

package brewrestart

import (
	"context"
	"net/netip"
)

// StopFixtureUpgradeHost substitutes only candidate discovery. Never included in
// release builds; native identity, listener and termination checks are unchanged.
func StopFixtureUpgradeHost(ctx context.Context, endpoint netip.AddrPort, pid int) error {
	return stopUpgradeHost(ctx, endpoint, []int{pid})
}
