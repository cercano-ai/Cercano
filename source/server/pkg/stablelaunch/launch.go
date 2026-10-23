// Package stablelaunch dispatches an explicitly enrolled stable executable to
// its verified selected member. It never enrolls, migrates, downloads or starts
// a background agent. Installation locators are not authority.
package stablelaunch

import (
	"cercano/source/server/internal/secrets"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/exclusion"
	"cercano/source/server/pkg/installcontext"
)

// Startup runs after private locator parsing/verification, before ordinary
// initialization. A verified child must never dispatch again. Absent enrollment
// leaves the old startup path unchanged. Invalid existing enrollment fails closed.
// Unix success replaces this process; Windows success returns the child's status.
func Startup(args []string, client bool, verified installcontext.Verified) (handled bool, code int, err error) {
	if err := secrets.ValidateEnvironment(); err != nil {
		return true, 1, err
	}
	if verified.Present() {
		return false, 0, nil
	}
	executable, e := os.Executable()
	if e != nil {
		return true, 1, e
	}
	// Check presence first: unenrolled copies must not derive/open updater state.
	if _, e = os.Lstat(filepath.Join(filepath.Dir(executable), enrollment.FileName)); os.IsNotExist(e) {
		return false, 0, nil
	} else if e != nil {
		return true, 1, e
	}
	root, e := installcontext.DefaultStateRoot()
	if e != nil {
		return true, 1, e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, e := prepare(ctx, executable, root, args, client)
	if e != nil {
		return true, 1, e
	}
	defer p.lock.Close()
	code, e = execute(p)
	return true, code, e
}

type prepared struct {
	target string
	args   []string
	lock   *exclusion.Handle
}

func prepare(ctx context.Context, executable, stateRoot string, args []string, client bool) (*prepared, error) {
	if ctx == nil {
		return nil, errors.New("stablelaunch: missing context")
	}
	// Do not accept a caller injecting a second installation identity.
	_, found, _, err := installcontext.Parse(args)
	if err != nil {
		return nil, err
	}
	if found {
		return nil, errors.New("stablelaunch: child locators cannot be redispatched")
	}
	enrolled, err := enrollment.DiscoverStableLauncher(ctx, executable)
	if err != nil {
		return nil, err
	}
	name := enrolled.Agent
	if client {
		name = enrolled.Client
	}
	if filepath.Base(executable) != name {
		return nil, enrollment.ErrUnsafe
	}
	locators := installcontext.Locators{InstallRoot: enrolled.Root, InstallID: enrolled.InstallID}
	checked, path, lock, err := installcontext.PrepareLaunch(ctx, locators, stateRoot, client)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = lock.Close()
		}
	}()
	if checked != enrolled || path == executable {
		return nil, enrollment.ErrUnsafe
	}
	current, err := enrollment.DiscoverStableLauncher(ctx, executable)
	if err != nil {
		return nil, err
	}
	if current != enrolled {
		return nil, enrollment.ErrUnsafe
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	private, err := installcontext.LocatorArgs(enrolled.Root, enrolled.InstallID)
	if err != nil {
		return nil, err
	}
	// Prepend locators so an ordinary -- terminator never hides them.
	childArgs := append(private, args...)
	keep = true
	return &prepared{target: path, args: childArgs, lock: lock}, nil
}

func launchError(err error) error {
	return fmt.Errorf("stablelaunch: selected executable could not start: %w", err)
}
