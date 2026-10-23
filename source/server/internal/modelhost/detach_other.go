//go:build !unix && !windows

package modelhost

import "syscall"

// detachSysProcAttr: this platform offers no session or console detachment
// through SysProcAttr, so the host is started with default attributes.
func detachSysProcAttr() *syscall.SysProcAttr {
	return nil
}
