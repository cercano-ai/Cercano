// Package setupreset implements a developer-triggered, best-effort setup reset.
// It intentionally neither coordinates nor stops sessions. Callers warn that
// concurrent work may fail or write old settings/credentials back afterward.
package setupreset

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/setupstate"
	"github.com/99designs/keyring"
	"gopkg.in/yaml.v3"
)

type Result struct {
	DeletedCredentials int
	LiveApplied        bool
	ConfigWritten      bool
	Config             config.Config
}
type Prepared struct {
	YAML   []byte
	Config config.Config
}

// Prepare reads only the requested configuration under the coordinator lock
// (which can create the lock file and parent directory). It does not publish
// configuration, access credentials or apply runtime state. Its returned
// document no longer contains legacy keys.
func Prepare(path string) (Prepared, error) {
	if err := regularOrMissing(path); err != nil {
		return Prepared{}, fmt.Errorf("configuration %q: check target path and permissions before retrying: %w", path, err)
	}
	coordinator, err := config.NewConfigCoordinator(path)
	if err != nil {
		return Prepared{}, fmt.Errorf("configuration %q: cannot resolve authority; check path and permissions", path)
	}
	base, err := coordinator.Read(context.Background())
	if err != nil {
		return Prepared{}, fmt.Errorf("configuration %q: cannot read valid configuration; check permissions and provide one valid YAML mapping, then retry", path)
	}
	output, err := base.SetupResetYAML()
	if err != nil {
		return Prepared{}, fmt.Errorf("configuration %q: cannot prepare reset; correct YAML syntax and schema then retry", path)
	}
	c := config.Defaults()
	if err := yaml.Unmarshal(output, &c); err != nil {
		return Prepared{}, fmt.Errorf("reset configuration has invalid types")
	}
	if err := c.LlamaServer.Validate(); err != nil {
		return Prepared{}, fmt.Errorf("reset runtime defaults are invalid")
	}
	return Prepared{YAML: output, Config: c.WithSnapshot(base)}, nil
}

func Reset(ctx context.Context, path string, store secrets.Store, applyLive func(config.Config) error) (Result, error) {
	return reset(ctx, path, store, applyLive, nil)
}

// publish is invocation-local fault injection for fake-store tests, never a
// process-wide replacement of the production coordinator.
func reset(ctx context.Context, path string, store secrets.Store, applyLive func(config.Config) error, publish func(context.Context, config.ConfigSnapshot) (config.ConfigSnapshot, error)) (Result, error) {
	var result Result
	if err := ctx.Err(); err != nil {
		return result, err
	}
	prepared, err := Prepare(path)
	if err != nil {
		return result, err
	}
	if store == nil {
		return result, fmt.Errorf("Cercano credential store is unavailable")
	}
	names, err := store.List()
	if err != nil {
		return result, fmt.Errorf("list Cercano credentials: %w", err)
	}
	// Publication is the admission boundary: rejected saves have no live or
	// credential effects. The following stores are deliberately not atomic.
	if err := ctx.Err(); err != nil {
		return result, err
	}
	coordinator, err := prepared.Config.PrepareConfigCoordination(path)
	if err != nil {
		return result, err
	}
	if publish == nil {
		publish = coordinator.ResetSetup
	}
	next, err := publish(ctx, prepared.Config.Snapshot())
	if err != nil {
		return result, fmt.Errorf("publish reset configuration %q; reload and reconcile before retrying: %w", path, err)
	}
	prepared.Config = prepared.Config.WithSnapshot(next)
	result.Config = prepared.Config
	result.ConfigWritten = true
	sort.Strings(names)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		err := store.Delete(name)
		if err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, keyring.ErrKeyNotFound) {
			return result, fmt.Errorf("delete Cercano credential after %d removals: %w", result.DeletedCredentials, err)
		}
		result.DeletedCredentials++
	}
	result.Config = prepared.Config
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if applyLive != nil {
		if err := applyLive(prepared.Config); err != nil {
			return result, fmt.Errorf("apply live reset configuration: %w", err)
		}
		result.LiveApplied = true
	}
	return result, nil
}

func WriteFreshWizard(path string) error {
	data, err := yaml.Marshal(setupstate.Fresh())
	if err != nil {
		return fmt.Errorf("encode fresh setup state: %w", err)
	}
	return writeAtomic(path, data)
}
func regularOrMissing(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("setup state requires an absolute file path")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("setup target must be a regular file, not a symlink or directory")
	}
	return nil
}
func writeAtomic(path string, data []byte) error {
	if err := regularOrMissing(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".setup-reset-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err := regularOrMissing(path); err != nil {
		return err
	}
	return os.Rename(name, path)
}
