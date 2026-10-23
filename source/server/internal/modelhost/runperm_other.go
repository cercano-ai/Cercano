//go:build !unix

package modelhost

import "os"

// Ownership and mode checks are skipped on platforms without POSIX owners.
func checkRunDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return os.ErrInvalid
	}
	return nil
}

func checkRecordFile(string) error { return nil }

const logOpenFlags = 0
