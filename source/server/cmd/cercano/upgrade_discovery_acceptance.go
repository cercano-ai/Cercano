//go:build ordinary_upgrade_acceptance

package main

import (
	"context"
	"fmt"
	"net/netip"
	"os"
)

// Disposable compiled acceptance only. Production has no environment bypass.
func upgradeAgentCheck(ctx context.Context) error {
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" {
		return fmt.Errorf("acceptance requires disabled credentials")
	}
	switch os.Getenv("CERCANO_ACCEPTANCE_UPGRADE_PROCESSES") {
	case "empty":
		return refuseKnownUpgradeAgents("", os.Getpid())
	case "second-active":
		path := os.Getenv("CERCANO_ACCEPTANCE_DISCOVERY_RECORD")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			f.Close()
			return refuseKnownUpgradeAgents("", os.Getpid())
		}
		return refuseKnownUpgradeAgents("999999 cercano", os.Getpid())
	case "active":
		return refuseKnownUpgradeAgents("999999 cercano", os.Getpid())
	default:
		return fmt.Errorf("explicit disposable process fixture required")
	}
}

func upgradeHostStop(ctx context.Context, endpoint netip.AddrPort) error {
	return acceptanceHostStop(ctx, endpoint)
}
