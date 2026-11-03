//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <sys/sysctl.h>
#include <sys/proc.h>
static int recovery_args(int pid, char *buf, size_t *size) {
 struct proc_bsdinfo a,b;
 errno=0;
 if(proc_pidinfo(pid,PROC_PIDTBSDINFO,0,&a,sizeof(a))!=sizeof(a)) return errno?errno:EAGAIN;
 if(a.pbi_status==SZOMB) return ESRCH;
 int mib[3]={CTL_KERN,KERN_PROCARGS2,pid};
 if(sysctl(mib,3,buf,size,NULL,0)!=0) {
  int code=errno;
  if(code==ESRCH) {
   errno=0;
   if(proc_pidinfo(pid,PROC_PIDTBSDINFO,0,&b,sizeof(b))==sizeof(b)) return b.pbi_status==SZOMB?ESRCH:EAGAIN;
   return errno==ESRCH?ESRCH:EAGAIN;
  }
  return code?code:EAGAIN;
 }
 errno=0;
 if(proc_pidinfo(pid,PROC_PIDTBSDINFO,0,&b,sizeof(b))!=sizeof(b)) return errno?errno:EAGAIN;
 if(a.pbi_start_tvsec!=b.pbi_start_tvsec || a.pbi_start_tvusec!=b.pbi_start_tvusec || a.pbi_uid!=b.pbi_uid) return EAGAIN;
 return 0;
}
*/
import "C"
import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

// VerifyHostAbsent checks every same-user kernel argv, including unlinked
// executables. The caller must hold the recorded host's launch exclusion lock.
// Any unreadable live process fails closed. No endpoint timeout or path lookup
// is used as death evidence; this never signals a process.
func VerifyHostAbsent(ctx context.Context, host string) error {
	return verifyHostAbsent(ctx, host, ListCandidates)
}

func verifyHostAbsent(ctx context.Context, host string, list func() ([]int, error)) error {
	if host == "" {
		return fmt.Errorf("exact source host required")
	}
	pids, err := list()
	if err != nil {
		return err
	}
	for _, pid := range pids {
		if err = ctx.Err(); err != nil {
			return err
		}
		buf := make([]byte, 1<<20)
		size := C.size_t(len(buf))
		code := syscall.Errno(C.recovery_args(C.int(pid), (*C.char)(unsafe.Pointer(&buf[0])), &size))
		if code == syscall.ESRCH {
			continue
		}
		if code != 0 {
			return fmt.Errorf("cannot verify source absence: process %d: %w", pid, code)
		}
		argv, e := recoveryArgv(buf[:int(size)])
		if e != nil {
			return e
		}
		for _, arg := range argv {
			if arg == host || arg == "--host="+host {
				return fmt.Errorf("source host still has a live process (%d)", pid)
			}
		}
	}
	return nil
}
func recoveryArgv(buf []byte) ([]string, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("short process arguments")
	}
	argc := int(binary.NativeEndian.Uint32(buf[:4]))
	buf = buf[4:]
	n := bytes.IndexByte(buf, 0)
	if n < 0 {
		return nil, fmt.Errorf("invalid process arguments")
	}
	buf = bytes.TrimLeft(buf[n+1:], "\x00")
	parts := bytes.Split(buf, []byte{0})
	if argc < 1 || argc > len(parts) {
		return nil, fmt.Errorf("invalid argc")
	}
	out := make([]string, argc)
	for i := range out {
		out[i] = string(parts[i])
	}
	return out, nil
}

// Arguments bind the already kernel-verified listener to the trusted host record;
// they never grant process authority alone.
func verifyLegacyHostArg(id Identity, host string) error {
	buf := make([]byte, 1<<20)
	size := C.size_t(len(buf))
	code := syscall.Errno(C.recovery_args(C.int(id.PID), (*C.char)(unsafe.Pointer(&buf[0])), &size))
	if code != 0 {
		return fmt.Errorf("source arguments unavailable: %w", code)
	}
	argv, err := recoveryArgv(buf[:int(size)])
	if err != nil {
		return err
	}
	count, matches := 0, 0
	for i, arg := range argv {
		if arg == "--host" || strings.HasPrefix(arg, "--host=") {
			count++
			if arg == "--host="+host || (arg == "--host" && i+1 < len(argv) && argv[i+1] == host) {
				matches++
			}
		}
	}
	now, err := Inspect(id.PID)
	if err != nil || !now.SameProcess(id) || count != 1 || matches != 1 {
		return fmt.Errorf("source host/process attribution changed")
	}
	return nil
}
