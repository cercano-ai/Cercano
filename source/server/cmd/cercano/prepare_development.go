package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/config"
)

// runPrepareDevelopment runs before normal startup. It copies only config YAML,
// without parsing it, resolving credentials, or opening legacy databases.
// Failure after creation leaves the new directory for inspection;
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
	// Refuse existing destinations before even opening the source. Mkdir below
	// remains the authoritative exclusive claim against concurrent provisioning.
	if _, err := os.Lstat(*root); !os.IsNotExist(err) {
		return fmt.Errorf("new-state must not exist")
	}
	// Reuse normal server discovery, not a second launcher-specific search path.
	// Copy raw bytes so credential references (and any inline secrets) remain
	// unchanged. Never call config.Load/Save or a native credential resolver.
	source, err := os.Open(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("source config required at %s; configure Cercano before preparing development state: %w", config.DefaultPath(), err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("source config must be a readable regular file")
	}
	// Mkdir, not MkdirAll: exclusive opt-in, even for an empty existing directory.
	if err := os.Mkdir(*root, 0700); err != nil {
		return fmt.Errorf("create new development state exclusively: %w", err)
	}
	destination, err := os.OpenFile(filepath.Join(*root, "config.yaml"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create private development config: %w", err)
	}
	_, copyErr := io.Copy(destination, source)
	if copyErr == nil {
		copyErr = destination.Sync()
	}
	closeErr := destination.Close()
	if copyErr != nil || closeErr != nil {
		return fmt.Errorf("copy source config failed; new state left for inspection at %s", *root)
	}
	if err := conversation.PrepareDevelopmentStore(ctx, filepath.Join(*root, "conversations.db")); err != nil {
		return fmt.Errorf("new state left unprepared for inspection at %s: %w", *root, err)
	}
	_, err = fmt.Fprintf(out, "Prepared new development store: %s\nExisting config copied privately; original unchanged. No legacy database or native credentials imported.\n", *root)
	return err
}
