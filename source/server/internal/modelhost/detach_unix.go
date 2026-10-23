//go:build unix

package modelhost

import "syscall"

// detachSysProcAttr detaches the spawned host from the caller's tty and
// process group (setsid) so it outlives the caller.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
