package main

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"cercano/source/server/internal/brewrestart"
	pb "cercano/source/server/pkg/proto"
)

// Caller holds the exact source launch lock. Credentials are loaded exclusively
// from its private registry record. No reconnect, launcher or optional new RPC
// participates; LifecycleList and ShutdownAgent are present in the pinned old host.
func (p *developmentLegacyStop) shutdownAuthenticated(ctx context.Context, host string, endpoint netip.AddrPort, expected brewrestart.Identity, validate func() error) error {
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
	return awaitLegacyShutdown(ctx, validate, func(ctx context.Context) (*pb.ShutdownAgentResponse, error) {
		return pb.NewAgentClient(conn).ShutdownAgent(ctx, &pb.ShutdownAgentRequest{Reason: "approved old-host conversation recovery"})
	}, func(ctx context.Context) error { return p.absent(ctx, host) })
}

// A response loss is deliberately blocking, even if the source subsequently dies:
// the coordinator must reconcile with the same approval/operation, never replay
// work. No newer status RPC or Complete field is required of historical hosts.
func awaitLegacyShutdown(ctx context.Context, validate func() error, request func(context.Context) (*pb.ShutdownAgentResponse, error), absent func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return fmt.Errorf("old source shutdown requires deadline")
	}
	if validate == nil {
		return fmt.Errorf("source shutdown requires approval")
	}
	if err := validate(); err != nil {
		return err
	}
	response, err := request(ctx)
	if err != nil {
		return fmt.Errorf("old source shutdown outcome uncertain: %w", err)
	}
	if !response.GetAccepted() {
		return fmt.Errorf("old source refused shutdown: %s", response.GetMessage())
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("old source shutdown not death-proven: %w", err)
		}
		err := absent(ctx)
		if err == nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("old source shutdown not death-proven (%v): %w", err, ctx.Err())
		case <-ticker.C:
		}
	}
}
