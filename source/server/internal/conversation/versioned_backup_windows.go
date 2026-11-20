package conversation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func backupOfflineWithSync(ctx context.Context, path string, syncFile func(*os.File) error) error {
	return backupOfflineWindows(ctx, path, syncFile, windows.MoveFileEx)
}

// Keep publication injectable without changing the production flags or relaxing
// path/ACL validation. The caller must not mutate the source until this returns.
func backupOfflineWindows(ctx context.Context, path string, syncFile func(*os.File) error, move func(*uint16, *uint16, uint32) error) (err error) {
	backup := path + ".pre-upgrade-backup"
	parent := filepath.Dir(backup)
	if err = validateWindowsCreationOwner(); err != nil {
		return err
	}
	h, err := windowsPrivateHandle(parent, true)
	if err != nil {
		return fmt.Errorf("validate offline backup directory: %w", err)
	}
	// Bound the supported durability contract to local NTFS. In particular, do
	// not silently accept remote shares, reparse points, or non-ACL filesystems.
	var filesystem [32]uint16
	err = windows.GetVolumeInformationByHandle(h, nil, 0, nil, nil, nil, &filesystem[0], uint32(len(filesystem)))
	windows.CloseHandle(h)
	if err != nil {
		return fmt.Errorf("query offline backup filesystem: %w", err)
	}
	if windows.UTF16ToString(filesystem[:]) != "NTFS" {
		return fmt.Errorf("offline backup publication requires local NTFS")
	}
	if _, err = os.Lstat(backup); err == nil {
		return fmt.Errorf("reserve offline upgrade backup: %w", os.ErrExist)
	} else if !os.IsNotExist(err) {
		return err
	}
	// os.CreateTemp creates exclusively in the already validated directory. Its
	// inherited descriptor is checked before SQLite writes any history into it;
	// mode 0600 alone does not provide Windows privacy. Never repair ACLs.
	staging, err := os.CreateTemp(parent, ".pre-upgrade-backup-*.staging")
	if err != nil {
		return err
	}
	name := staging.Name()
	defer func() {
		if err != nil {
			err = fmt.Errorf("offline backup failed (recovery staging retained at %q): %w", name, err)
		}
	}()
	if err = staging.Close(); err != nil {
		return err
	}
	h, err = windowsPrivateHandle(name, false)
	if err != nil {
		return err
	}
	windows.CloseHandle(h)
	reader, err := openCoexistenceDB(path, "ro")
	if err != nil {
		return err
	}
	_, err = reader.ExecContext(ctx, "VACUUM INTO ?", name)
	closeErr := reader.Close()
	if err != nil {
		return fmt.Errorf("save offline upgrade snapshot: %w", err)
	}
	if closeErr != nil {
		return closeErr
	}
	saved, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = syncFile(saved)
	closeErr = saved.Close()
	if err != nil {
		return fmt.Errorf("flush offline upgrade snapshot: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close offline upgrade snapshot: %w", closeErr)
	}
	from, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(backup)
	if err != nil {
		return err
	}
	// MoveFileExW documents WRITE_THROUGH as not returning until the file is
	// actually moved on disk. Same-directory move; deliberately NO
	// REPLACE_EXISTING, COPY_ALLOWED, or delayed-reboot fallback. A destination
	// racing the precheck therefore fails closed, preserving both artifacts.
	// https://learn.microsoft.com/windows/win32/api/winbase/nf-winbase-movefileexw
	// This API contract is not a claim of hardware/power-loss qualification.
	if err = move(from, to, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("publish offline upgrade backup: %w", err)
	}
	return nil
}
