//go:build ordinary_upgrade_acceptance

package main

import (
	"context"
	"fmt"
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
	case "active":
		return refuseKnownUpgradeAgents("999999 cercano", os.Getpid())
	default:
		return fmt.Errorf("explicit disposable process fixture required")
	}
}
