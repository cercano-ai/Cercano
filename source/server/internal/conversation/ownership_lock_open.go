package conversation

import (
	"crypto/sha256"
	"fmt"
	"os"
)

// Every capability entry point uses the same path, privacy and handle checks.
// Stable lock files must never be removed or replaced while the store is in use.
func openOwnershipLock(database, id string, create bool) (*os.File, error) {
	if err := validateOwnershipPath(database); err != nil {
		return nil, err
	}
	if err := validateCoexistencePrivacy(database); err != nil {
		return nil, err
	}
	dir := database + ".ownership-locks"
	if create {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	if err := validateLockDirectory(dir); err != nil {
		return nil, err
	}
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	f, err := os.OpenFile(fmt.Sprintf("%s/%x.lock", dir, sha256.Sum256([]byte(id))), flags, 0600)
	if err != nil {
		return nil, err
	}
	if err := validateLockHandle(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
