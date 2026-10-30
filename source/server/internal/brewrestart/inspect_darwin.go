//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <string.h>

// A failed path lookup alone does not establish that the PID disappeared.
static int cercano_identity_exists(int pid) {
 struct proc_bsdinfo info;
 errno = 0;
 if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, sizeof(info)) != sizeof(info))
  return errno ? errno : EAGAIN;
 return 0;
}

// Read identity twice around the path query to reject mixed snapshots.
static int cercano_identity(int pid, struct proc_bsdinfo *info, char *path) {
 struct proc_bsdinfo before;
 memset(&before, 0, sizeof(before));
 memset(info, 0, sizeof(*info));
 errno = 0;
 if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &before, sizeof(before)) != sizeof(before))
  return errno ? errno : ESRCH;
 errno = 0;
 if (proc_pidpath(pid, path, PROC_PIDPATHINFO_MAXSIZE) <= 0)
  return errno ? errno : ESRCH;
 errno = 0;
 if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, info, sizeof(*info)) != sizeof(*info))
  return errno ? errno : ESRCH;
 if (before.pbi_start_tvsec != info->pbi_start_tvsec ||
     before.pbi_start_tvusec != info->pbi_start_tvusec ||
     before.pbi_uid != info->pbi_uid) return EAGAIN;
 return 0;
}
*/
import "C"

import (
	"fmt"
	"syscall"
)

// Inspect reads a single process's identity using libproc. It neither connects
// to an agent nor sends signals. Callers must revalidate before acting and
// separately verify which process owns the target socket.
func Inspect(pid int) (Identity, error) {
	if pid <= 0 || int64(pid) > 2147483647 {
		return Identity{}, fmt.Errorf("invalid PID %d", pid)
	}
	var info C.struct_proc_bsdinfo
	var path [C.PROC_PIDPATHINFO_MAXSIZE]C.char
	if code := C.cercano_identity(C.int(pid), &info, &path[0]); code != 0 {
		err := confirmedInspectionError(syscall.Errno(code), func() syscall.Errno {
			return syscall.Errno(C.cercano_identity_exists(C.int(pid)))
		})
		return Identity{}, fmt.Errorf("inspect PID %d: %w", pid, err)
	}
	return Identity{
		PID: pid, ParentPID: int(info.pbi_ppid), ProcessGroup: int(info.pbi_pgid), UID: uint32(info.pbi_uid), Executable: C.GoString(&path[0]),
		StartSeconds: uint64(info.pbi_start_tvsec), StartMicroseconds: uint64(info.pbi_start_tvusec),
	}, nil
}

// ENOENT can describe a live process whose executable was unlinked, not a
// missing PID. Only a fresh kernel ESRCH establishes disappearance. Permission
// failures, short snapshots and PID reuse remain failures, never absence.
func confirmedInspectionError(code syscall.Errno, probe func() syscall.Errno) error {
	if code == syscall.ENOENT && probe() == syscall.ESRCH {
		return syscall.ESRCH
	}
	return code
}
