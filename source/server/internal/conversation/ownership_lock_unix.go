//go:build unix

package conversation

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func lockOwnershipFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

// Path-derived stable lock identities cannot safely support hard-link aliases.
// The fixture and lock directory must not be renamed/replaced while in use.
func validateOwnershipPath(path string) error {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return err
	}
	if st.Nlink != 1 {
		return fmt.Errorf("coexistence hard-linked database paths unsupported")
	}
	return nil
}
