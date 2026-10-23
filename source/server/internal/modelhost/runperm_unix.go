//go:build unix

package modelhost

import (
	"fmt"
	"os"
	"syscall"
)

// checkRunDir rejects a run directory that is not a real directory owned by
// the current user, and tightens one that is ours but group/other accessible.
func checkRunDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("model host run dir %s is not a plain directory", dir)
	}
	if err := ownedByUs(fi); err != nil {
		return fmt.Errorf("model host run dir %s: %w", dir, err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("tightening model host run dir %s: %w", dir, err)
		}
	}
	return nil
}

// checkRecordFile rejects a record that is a symlink or not owned by us.
func checkRecordFile(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("model host record %s is not a regular file", path)
	}
	if err := ownedByUs(fi); err != nil {
		return fmt.Errorf("model host record %s: %w", path, err)
	}
	return nil
}

func ownedByUs(fi os.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(st.Uid) != os.Getuid() {
		return fmt.Errorf("owned by uid %d, not the current user", st.Uid)
	}
	return nil
}

// logOpenFlags refuses to follow a symlink planted at the log path.
const logOpenFlags = syscall.O_NOFOLLOW
