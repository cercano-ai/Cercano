package installcontext

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/state"
)

// DefaultStateRoot derives the per-user updater state base directory exactly
// the way the agent's installed-TUI startup does (cmd/cercano/
// tui_update_notices.go): the home directory plus the platform-specific
// environment probes. It is pure derivation from probes; the locators never
// influence it.
func DefaultStateRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("installcontext: home directory: %w", err)
	}
	root, err := state.StateRoot(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_STATE_HOME"))
	if err != nil {
		return "", fmt.Errorf("installcontext: updater state root: %w", err)
	}
	return root, nil
}

// VerifyChild validates only protected enrollment, selection and exact layout.
// Locators are not authority; updater bookkeeping is never opened.
func VerifyChild(ctx context.Context, locators Locators, stateRoot, executable string, client bool) (enrollment.Record, error) {
	record, path, lock, err := PrepareLaunch(ctx, locators, stateRoot, client)
	if err != nil {
		return enrollment.Record{}, err
	}
	defer lock.Close()
	if executable != path {
		return enrollment.Record{}, enrollment.ErrUnsafe
	}
	return record, nil
}

// VerifyStartupVerified is the early main() gate for a child launched with
// the approved locators: it validates the ACTUAL running executable
// (os.Executable(), never a caller- or locator-supplied path) and the fixed
// launcher role against the installation's protected selection,
// using the per-user state root derived from the environment. It must run
// before any normal flag parsing, config load, DB open, provider or agent
// start. It performs exactly ONE verification pass and returns the opaque
// verified installation context for consumers that need it (stable launcher,
// enrollment identity, role); on failure the returned context is absent and
// paired with the error, so a caller cannot keep a half-verified identity.
// An error means the process must refuse startup; without locators startup
// is unchanged and this is never called.
func VerifyStartupVerified(ctx context.Context, locators Locators, client bool) (Verified, error) {
	executable, err := os.Executable()
	if err != nil {
		return Verified{}, fmt.Errorf("installcontext: running executable: %w", err)
	}
	stateRoot, err := DefaultStateRoot()
	if err != nil {
		return Verified{}, err
	}
	return VerifyChildVerified(ctx, locators, stateRoot, executable, client)
}

// VerifyStartup keeps the bare verified record for existing callers; it is
// the same single-pass startup gate as VerifyStartupVerified.
func VerifyStartup(ctx context.Context, locators Locators, client bool) (enrollment.Record, error) {
	verified, err := VerifyStartupVerified(ctx, locators, client)
	if err != nil {
		return enrollment.Record{}, err
	}
	return verified.Record(), nil
}
