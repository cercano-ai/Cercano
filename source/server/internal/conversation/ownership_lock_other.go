//go:build !unix

package conversation

import (
	"fmt"
	"os"
)

func lockOwnershipFile(f *os.File) error {
	return fmt.Errorf("coexistence ownership OS locks unsupported on this platform")
}

func validateOwnershipPath(path string) error {
	return fmt.Errorf("coexistence ownership OS locks unsupported on this platform")
}
