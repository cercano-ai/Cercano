//go:build !linux && !(darwin && cgo)

package brewrestart

import (
	"errors"
	"net/netip"
)

// ListenerProcess is not available on this platform.
func ListenerProcess(netip.AddrPort) (Identity, bool, error) {
	return Identity{}, false, errors.ErrUnsupported
}

// ProcessExited is not available on this platform.
func ProcessExited(Identity) (bool, error) { return false, errors.ErrUnsupported }
