package main

import (
	"context"
	"fmt"
	"net/netip"

	"cercano/source/server/internal/brewrestart"
)

// Caller holds the exact source launch lock. Credentials are loaded exclusively
// from its private registry record. No reconnect, launcher or optional new RPC
// participates. Authenticate the pinned old host before native termination.
func (p *developmentContainedSource) shutdownAuthenticated(ctx context.Context, host string, endpoint netip.AddrPort, expected brewrestart.Identity, validate func() error) error {
	selected, err := selectDevelopmentPeer(p.state, p.destination, "", host)
	if err != nil {
		return err
	}
	conn, err := developmentPeerDial(p.state, p.destination)(ctx, endpoint.String(), selected.Identity)
	if err != nil {
		return fmt.Errorf("authenticate old source: %w", err)
	}
	defer conn.Close()
	// Recheck the kernel listener after authentication, before consuming consent.
	actual, err := p.inspect(ctx, host, endpoint)
	if err != nil {
		return err
	}
	if !actual.SameProcess(expected) {
		return fmt.Errorf("authenticated source process changed")
	}
	// Fresh confirmation authorizes bounded TERM/KILL of this exact process,
	// not an optional RPC whose drain can retain the very work blocking resume.
	// StopLegacyHost rechecks process incarnation and consent before signalling.
	return p.terminate(ctx, host, endpoint, expected, validate)
}
