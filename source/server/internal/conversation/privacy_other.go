//go:build !windows

package conversation

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func validatePrivateDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("development store parent must be private")
	}
	return nil
}
func validateCoexistencePrivacy(string) error { return nil }
func validateLockDirectory(string) error      { return nil }
func validateLockHandle(*os.File) error       { return nil }
func ownershipLockContended(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}
