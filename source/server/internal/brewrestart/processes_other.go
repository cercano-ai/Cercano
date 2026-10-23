//go:build !linux && !(darwin && cgo)

package brewrestart

import "errors"

// ListUserProcesses is not available on this platform.
func ListUserProcesses(func(UserProcess) bool) ([]UserProcess, error) {
	return nil, errors.ErrUnsupported
}
