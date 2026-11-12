//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <stdint.h>
#include <stddef.h>
#include <dlfcn.h>
#include <libproc.h>
#include <mach/message.h>

// XNU bsd/sys/proc_info_private.h: PROC_PIDUNIQIDENTIFIERINFO (17).
// The kernel declares this 56-byte structure an API. Keep the private ABI
// isolated here; an unsupported flavor or short read fails closed.
// https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/proc_info_private.h
struct cercano_proc_unique {
 uint8_t uuid[16];
 uint64_t uniqueid, parent_uniqueid;
 int32_t version, parent_version;
 uint64_t reserved2, reserved3;
};
_Static_assert(sizeof(struct cercano_proc_unique) == 56, "process identity ABI size");
_Static_assert(offsetof(struct cercano_proc_unique, version) == 32, "process identity ABI offset");
static int cercano_pid_version(int pid, uint32_t *version) {
 struct cercano_proc_unique info = {0};
 errno = 0;
 int n = proc_pidinfo(pid, 17, 0, &info, sizeof(info));
 if (n != sizeof(info)) return errno ? errno : EIO;
 *version = (uint32_t)info.version;
 return 0;
}
static int cercano_signal_token(int pid, uint32_t version, int sig) {
 // Resolve dynamically so older systems fail closed, never fall back to kill.
 typedef int (*signal_fn)(audit_token_t *, int);
 signal_fn send = (signal_fn)dlsym(RTLD_DEFAULT, "proc_signal_with_audittoken");
 if (!send) return ENOSYS;
 audit_token_t token = {{0}};
 token.val[5] = (uint32_t)pid;
 token.val[7] = version;
 // XNU proc_find_audit_token validates PID + pidversion; credentials for
 // permission checks are those of the caller, not these token fields.
 // psignal_by_audit_token retains the exact process through delivery.
 // libproc returns an errno value directly, not -1 with errno.
 return send(&token, sig);
}
*/
import "C"

import "syscall"

func processVersion(pid int) (uint32, error) {
	var version C.uint32_t
	if code := C.cercano_pid_version(C.int(pid), &version); code != 0 {
		return 0, syscall.Errno(code)
	}
	return uint32(version), nil
}

func signalProcessVersion(pid int, version uint32, sig syscall.Signal) error {
	if code := C.cercano_signal_token(C.int(pid), C.uint32_t(version), C.int(sig)); code != 0 {
		return syscall.Errno(code)
	}
	return nil
}
