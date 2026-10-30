//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <sys/sysctl.h>
static int missing_info(int pid, struct proc_bsdinfo *info) {
 errno=0;
 if(proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, info, sizeof(*info)) != sizeof(*info)) return errno ? errno : EAGAIN;
 return 0;
}
static int missing_args(int pid, char *buf, size_t *size) {
 int mib[3]={CTL_KERN,KERN_PROCARGS2,pid};
 errno=0;
 if(sysctl(mib,3,buf,size,NULL,0)) return errno ? errno : EAGAIN;
 return 0;
}
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// missingExecutableEvidence reads the kernel's saved exec path, NOT argv[0].
// It is exclusion evidence only: never an identity accepted for signalling.
// A stable lifetime and repeated saved path bracket the socket snapshot.
func missingExecutableEvidence(pid int) (Identity, error) {
	var before, after C.struct_proc_bsdinfo
	if code := C.missing_info(C.int(pid), &before); code != 0 {
		return Identity{}, syscall.Errno(code)
	}
	buf := make([]byte, 1<<20)
	size := C.size_t(len(buf))
	if code := C.missing_args(C.int(pid), (*C.char)(unsafe.Pointer(&buf[0])), &size); code != 0 {
		return Identity{}, syscall.Errno(code)
	}
	if size < 5 || int(size) > len(buf) {
		return Identity{}, fmt.Errorf("invalid saved exec path")
	}
	raw := buf[4:int(size)]
	n := bytes.IndexByte(raw, 0)
	if n <= 0 || !filepath.IsAbs(string(raw[:n])) {
		return Identity{}, fmt.Errorf("unresolved saved exec path")
	}
	if code := C.missing_info(C.int(pid), &after); code != 0 {
		return Identity{}, syscall.Errno(code)
	}
	if before.pbi_start_tvsec != after.pbi_start_tvsec || before.pbi_start_tvusec != after.pbi_start_tvusec || before.pbi_uid != after.pbi_uid {
		return Identity{}, fmt.Errorf("candidate lifetime changed")
	}
	// The saved launch path can be an alias, not the resolved executable. Kernel
	// names can add refusal evidence but never confer ownership or signal authority.
	for _, name := range []string{C.GoString(&after.pbi_name[0]), C.GoString(&after.pbi_comm[0])} {
		if name == "cercano" || name == "cercano-agent" {
			return Identity{}, fmt.Errorf("unresolved plausible writer: kernel executable name %s", name)
		}
	}
	if _, err := os.Lstat(string(raw[:n])); !errors.Is(err, os.ErrNotExist) {
		return Identity{}, fmt.Errorf("unresolved plausible writer: saved launch path still exists or is inaccessible (possible alias or replacement)")
	}
	// Do not apply this evidence after an exec has made normal inspection work.
	if _, err := Inspect(pid); !errors.Is(err, syscall.ENOENT) {
		return Identity{}, fmt.Errorf("candidate executable state changed: %v", err)
	}
	return Identity{PID: pid, ParentPID: int(after.pbi_ppid), ProcessGroup: int(after.pbi_pgid), UID: uint32(after.pbi_uid), Executable: string(raw[:n]), StartSeconds: uint64(after.pbi_start_tvsec), StartMicroseconds: uint64(after.pbi_start_tvusec)}, nil
}

func inspectUpgradeCandidate(pid int, endpoint netip.AddrPort) (Identity, error) {
	id, err := Inspect(pid)
	if !errors.Is(err, syscall.ENOENT) {
		return id, err
	}
	evidence, evidenceErr := missingExecutableEvidence(pid)
	if evidenceErr != nil {
		return Identity{}, fmt.Errorf("upgrade blocker pid=%d: missing executable; candidate unclassified: %w", pid, evidenceErr)
	}
	name := filepath.Base(evidence.Executable)
	if name == "cercano" || name == "cercano-agent" {
		return Identity{}, fmt.Errorf("upgrade blocker pid=%d: missing executable %s; unresolved plausible writer (including model-host); stop it explicitly", pid, evidence.Executable)
	}
	owns, socketErr := holdsListenerInspected(evidence, endpoint, missingExecutableEvidence)
	if socketErr != nil {
		return Identity{}, fmt.Errorf("upgrade blocker pid=%d: missing executable; listener unresolved: %w", pid, socketErr)
	}
	if owns {
		return Identity{}, fmt.Errorf("upgrade blocker pid=%d: missing executable %s owns target listener; unresolved plausible writer", pid, evidence.Executable)
	}
	// Neither a bounded known writer nor a target listener. No signal authority.
	return Identity{}, nil
}
