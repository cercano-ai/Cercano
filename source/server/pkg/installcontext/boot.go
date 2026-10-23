package installcontext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/exclusion"
	"cercano/source/server/internal/updatecoord/privdir"
	"cercano/source/server/internal/updatecoord/selection"
	"cercano/source/server/internal/updatecoord/state"
)

var ErrLaunchBusy = errors.New("update in progress; retry launch shortly")

// BootDirectory derives the EXISTING state/selection/lock location without SQL.
// It does not provision or repair any directory.
func BootDirectory(root, id string) (string, error) {
	if err := state.ValidateInstallID(id); err != nil {
		return "", err
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", enrollment.ErrUnsafe
	}
	org := "Cercano"
	if runtime.GOOS == "linux" {
		org = "cercano"
	}
	dir := filepath.Join(root, org, "updater", id)
	var err error
	for _, d := range []string{filepath.Dir(filepath.Dir(dir)), filepath.Dir(dir), dir} {
		if err = privdir.VerifyExisting(d); err != nil {
			return "", err
		}
		canonical, e := filepath.EvalSymlinks(d)
		if e != nil || canonical != d {
			return "", enrollment.ErrUnsafe
		}
	}
	return dir, nil
}

// PrepareLaunch validates one selected role under one short shared OS lease.
// The caller owns the returned lease: close after admission, or let Unix exec
// close its CLOEXEC descriptor. Never nest verification on this handle.
func PrepareLaunch(ctx context.Context, loc Locators, root string, client bool) (record enrollment.Record, target string, lease *exclusion.Handle, err error) {
	if ctx == nil {
		err = enrollment.ErrUnsafe
		return
	}
	if err = loc.Validate(); err != nil {
		return
	}
	record, err = enrollment.Read(ctx, loc.InstallRoot)
	if err != nil {
		return
	}
	if record.InstallID != loc.InstallID {
		err = enrollment.ErrUnsafe
		return
	}
	dir, e := BootDirectory(root, loc.InstallID)
	if e != nil {
		err = e
		return
	}
	dirBefore, e := os.Lstat(dir)
	if e != nil {
		err = e
		return
	}
	bounded, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	lease, err = exclusion.Acquire(bounded, dir, exclusion.Launch)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			err = ErrLaunchBusy
		}
		return
	}
	defer func() {
		if err != nil {
			lease.Close()
			lease = nil
		}
	}()
	// Acquire proves its opened lock and directory identity. Keep that named
	// world stable through validation as well as acquisition.
	lockBefore, e := os.Lstat(filepath.Join(dir, "update.lock"))
	if e != nil {
		err = e
		return
	}
	selectionPath := filepath.Join(dir, selection.FileName)
	selected, e := readBootSelection(selectionPath)
	if e != nil {
		err = e
		return
	}
	if selected.InstallID != record.InstallID {
		err = enrollment.ErrUnsafe
		return
	}
	name := record.Agent
	if client {
		name = record.Client
	}
	versions := filepath.Join(record.Root, "versions")
	version := filepath.Join(versions, filepath.FromSlash(selected.StagedVersionDir))
	bin := filepath.Join(version, "bin")
	// Validate every ancestor, including any nested staged identifier.
	for d := bin; ; d = filepath.Dir(d) {
		if err = privdir.VerifyExisting(d); err != nil {
			return
		}
		canonical, e := filepath.EvalSymlinks(d)
		if e != nil || canonical != d {
			err = enrollment.ErrUnsafe
			return
		}
		if d == versions {
			break
		}
	}
	target = filepath.Join(bin, name)
	if err = verifyBootImage(target); err != nil {
		return
	}
	after, e := readBootSelection(selectionPath)
	if e != nil {
		err = e
		return
	}
	current, e := enrollment.Read(ctx, loc.InstallRoot)
	if e != nil {
		err = e
		return
	}
	if after != selected || current != record {
		err = enrollment.ErrUnsafe
		return
	}
	dirAfter, e := os.Lstat(dir)
	if e != nil {
		err = e
		return
	}
	lockAfter, e := os.Lstat(filepath.Join(dir, "update.lock"))
	if e != nil {
		err = e
		return
	}
	if !dirAfter.IsDir() || !os.SameFile(dirBefore, dirAfter) ||
		!lockAfter.Mode().IsRegular() || !os.SameFile(lockBefore, lockAfter) {
		err = enrollment.ErrUnsafe
		return
	}
	err = ctx.Err()
	return
}

func regularBootImage(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return nil, enrollment.ErrUnsafe
	}
	return info, nil
}
