//go:build developmentfixture && cercano_development_acceptance

// Acceptance-only operation driver. The harness owns a fresh private database,
// closes its writer, and launches this before any application hosts. Production
// upgrade-store always retains its process-discovery preflight; this driver is
// not a user-facing bypass and is absent from normal builds.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cercano/source/server/internal/conversation"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" || len(os.Args) != 2 || !filepath.IsAbs(os.Args[1]) {
		return fmt.Errorf("requires disabled credentials and absolute private fixture database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := conversation.UpgradeOffline(ctx, os.Args[1]); err != nil {
		return err
	}
	fmt.Println("Real UpgradeOffline completed; process discovery intentionally not exercised")
	return nil
}
