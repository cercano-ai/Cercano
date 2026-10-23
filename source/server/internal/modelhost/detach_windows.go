//go:build windows

package modelhost

import "syscall"

// detachedProcess is DETACHED_PROCESS, which package syscall does not define.
const detachedProcess = 0x00000008

// detachSysProcAttr starts the host without a console (DETACHED_PROCESS) and
// in its own process group (CREATE_NEW_PROCESS_GROUP), so closing the
// caller's console or pressing Ctrl+C there does not reach the host, which
// must outlive the caller.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess}
}
