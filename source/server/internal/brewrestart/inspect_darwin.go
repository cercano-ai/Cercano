//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <string.h>

// Read the identity twice around the executable-path query. A disappearing
// process, credential change or PID reuse must never produce a mixed snapshot.
// With allow_gone, a path the kernel cannot give (the executable or its
// directory was removed) sets *gone instead of failing.
static int cercano_identity(int pid, struct proc_bsdinfo *info, char *path, int allow_gone, int *gone) {
 struct proc_bsdinfo before;
 memset(&before, 0, sizeof(before));
 memset(info, 0, sizeof(*info));
 *gone = 0;
 errno = 0;
 if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &before, sizeof(before)) != sizeof(before))
  return errno ? errno : ESRCH;
 errno = 0;
 if (proc_pidpath(pid, path, PROC_PIDPATHINFO_MAXSIZE) <= 0) {
  int e = errno ? errno : ESRCH;
  path[0] = 0;
  if (!allow_gone || e == ESRCH) return e;
  *gone = 1;
 }
 if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, info, sizeof(*info)) != sizeof(*info))
  return errno ? errno : ESRCH;
 if (before.pbi_start_tvsec != info->pbi_start_tvsec ||
     before.pbi_start_tvusec != info->pbi_start_tvusec ||
     before.pbi_uid != info->pbi_uid) return EAGAIN;
 return 0;
}

static void cercano_identity_name(const struct proc_bsdinfo *info, char *name) {
 const char *src = info->pbi_name[0] ? info->pbi_name : info->pbi_comm;
 size_t max = info->pbi_name[0] ? sizeof(info->pbi_name) : sizeof(info->pbi_comm);
 size_t n = strnlen(src, max);
 memcpy(name, src, n);
 name[n] = 0;
}
*/
import "C"

import (
	"fmt"
	"syscall"
)

// Inspect reads a single process's identity using libproc. It neither connects
// to an agent nor sends signals. Callers must revalidate before acting and
// separately verify which process owns the target socket. It fails when the
// kernel cannot give the executable's path.
func Inspect(pid int) (Identity, error) {
	return inspectProcess(pid, false)
}

// inspectProcess is Inspect that, with allowGone, also returns a process
// whose executable path is gone, marked ExecutableGone and named by Name.
func inspectProcess(pid int, allowGone bool) (Identity, error) {
	if pid <= 0 || int64(pid) > 2147483647 {
		return Identity{}, fmt.Errorf("invalid PID %d", pid)
	}
	var info C.struct_proc_bsdinfo
	var path [C.PROC_PIDPATHINFO_MAXSIZE]C.char
	var name [2*C.MAXCOMLEN + 1]C.char
	var allow, gone C.int
	if allowGone {
		allow = 1
	}
	if code := C.cercano_identity(C.int(pid), &info, &path[0], allow, &gone); code != 0 {
		return Identity{}, fmt.Errorf("inspect PID %d: %w", pid, syscall.Errno(code))
	}
	C.cercano_identity_name(&info, &name[0])
	return Identity{
		PID: pid, UID: uint32(info.pbi_uid), Executable: C.GoString(&path[0]),
		ExecutableGone: gone != 0, Name: C.GoString(&name[0]),
		StartSeconds: uint64(info.pbi_start_tvsec), StartMicroseconds: uint64(info.pbi_start_tvusec),
	}, nil
}
