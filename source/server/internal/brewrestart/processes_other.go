//go:build !linux && !(darwin && cgo)

package brewrestart

import "errors"

// ListUserProcesses is not available on this platform.
func ListUserProcesses(func(executable string) bool) ([]UserProcess, error) {
	return nil, errors.ErrUnsupported
}
