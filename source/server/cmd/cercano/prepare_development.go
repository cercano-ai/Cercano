package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"cercano/source/server/internal/conversation"
)

// runPrepareDevelopment runs before normal startup. It never reads configuration,
// credentials or legacy databases. Failure leaves the new directory for inspection;
// neither retries nor cleanup may adopt or remove an existing destination.
func runPrepareDevelopment(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("prepare-development", flag.ContinueOnError)
	fs.SetOutput(out)
	root := fs.String("new-state", "", "absolute NEW separate development directory (never overwritten)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || !filepath.IsAbs(*root) || filepath.Clean(*root) != *root {
		return fmt.Errorf("prepare-development requires --new-state /absolute/new/separate/directory")
	}
	parent := filepath.Dir(*root)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	if resolved != parent {
		return fmt.Errorf("new-state parent must not contain symlink aliases")
	}
	// Mkdir, not MkdirAll: exclusive opt-in, even for an empty existing directory.
	if err := os.Mkdir(*root, 0700); err != nil {
		return fmt.Errorf("create new development state exclusively: %w", err)
	}
	if err := conversation.PrepareDevelopmentStore(ctx, filepath.Join(*root, "conversations.db")); err != nil {
		return fmt.Errorf("new state left unprepared for inspection at %s: %w", *root, err)
	}
	_, err = fmt.Fprintf(out, "Prepared new development store: %s\nNo legacy state imported or changed.\n", *root)
	return err
}
