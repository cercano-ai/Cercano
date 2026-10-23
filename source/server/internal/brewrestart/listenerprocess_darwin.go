//go:build darwin && cgo

package brewrestart

import (
	"errors"
	"net/netip"
	"syscall"

	"golang.org/x/sys/unix"
)

// ListenerProcess scans same-user processes with libproc for the holder of
// a listener accepting connections to endpoint. A holder whose executable
// was removed is returned with ExecutableGone and its Name.
func ListenerProcess(endpoint netip.AddrPort) (Identity, bool, error) {
	pids, err := ListCandidates()
	if err != nil {
		return Identity{}, false, err
	}
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		id, err := inspectProcess(pid, true)
		if err != nil {
			continue // exited meanwhile, or not inspectable
		}
		holds, err := HoldsListener(id, endpoint)
		if err != nil || !holds {
			continue
		}
		return id, true, nil
	}
	return Identity{}, false, nil
}

// sZomb is SZOMB from <sys/proc.h>.
const sZomb = 5

// ProcessExited reads the kernel's process entry: none, a zombie, or a
// different start time all mean the original process is gone.
func ProcessExited(id Identity) (bool, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", id.PID)
	if err != nil {
		if _, ierr := inspectProcess(id.PID, true); errors.Is(ierr, syscall.ESRCH) {
			return true, nil
		}
		return false, err
	}
	if int(kp.Proc.P_pid) != id.PID || kp.Proc.P_stat == sZomb {
		return true, nil
	}
	start := kp.Proc.P_starttime
	return uint64(start.Sec) != id.StartSeconds || uint64(start.Usec) != id.StartMicroseconds, nil
}
