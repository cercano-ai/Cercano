//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <sys/sysctl.h>
#include <stdlib.h>
#include <string.h>
#include <sys/un.h>

// Complete socket inventory: a matching bound Unix socket and no Internet listener.
static int missing_worker_sockets(int pid, const char *path, int *matched, int *listener) {
 errno=0;
 int n=proc_pidinfo(pid,PROC_PIDLISTFDS,0,NULL,0);
 if(n<=0) return errno ? errno : EAGAIN;
 if(n>16*1024*1024) return EOVERFLOW;
 int capacity=n+32*sizeof(struct proc_fdinfo);
 struct proc_fdinfo *fds=calloc(1,capacity);
 if(!fds) return ENOMEM;
 n=proc_pidinfo(pid,PROC_PIDLISTFDS,0,fds,capacity);
 if(n<=0 || n>=capacity || n%sizeof(*fds)) {int e=errno ? errno : EAGAIN;free(fds);return e;}
 *matched=0; *listener=0;
 for(int i=0;i<n/sizeof(*fds);i++) {
  if(fds[i].proc_fdtype!=PROX_FDTYPE_SOCKET) continue;
  struct socket_fdinfo s; memset(&s,0,sizeof(s)); errno=0;
  if(proc_pidfdinfo(pid,fds[i].proc_fd,PROC_PIDFDSOCKETINFO,&s,sizeof(s))!=sizeof(s)) {int e=errno ? errno : EAGAIN;free(fds);return e;}
  if((s.psi.soi_family==AF_INET || s.psi.soi_family==AF_INET6) && s.psi.soi_kind==SOCKINFO_TCP && s.psi.soi_proto.pri_tcp.tcpsi_state==TSI_S_LISTEN) *listener=1;
  if(s.psi.soi_family==AF_UNIX && s.psi.soi_kind==SOCKINFO_UN && s.psi.soi_type==SOCK_STREAM && !strncmp(s.psi.soi_proto.pri_un.unsi_addr.ua_sun.sun_path,path,sizeof(s.psi.soi_proto.pri_un.unsi_addr.ua_sun.sun_path))) *matched=1;
 }
 free(fds); return 0;
}
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
	"encoding/binary"
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
	// Names alone cannot distinguish the host (direct DB writer) from its
	// worker (persistence messages go to the host). Arguments alone cannot either.
	plausible := filepath.Base(string(raw[:n])) == "cercano" || filepath.Base(string(raw[:n])) == "cercano-agent"
	for _, name := range []string{C.GoString(&after.pbi_name[0]), C.GoString(&after.pbi_comm[0])} {
		plausible = plausible || name == "cercano" || name == "cercano-agent"
	}
	if plausible {
		socket := missingWorkerSocket(buf[:int(size)])
		if socket == "" {
			return Identity{}, fmt.Errorf("unresolved plausible writer: no corroborated worker role")
		}
		path := C.CString(socket)
		defer C.free(unsafe.Pointer(path))
		var matched, listener C.int
		if code := C.missing_worker_sockets(C.int(pid), path, &matched, &listener); code != 0 {
			return Identity{}, missingListenerError(pid, syscall.Errno(code), missingIdentityError)
		}
		if err := missingWorkerSocketResult(matched != 0, listener != 0); err != nil {
			return Identity{}, err
		}
		// Repeat the entire saved path/argv snapshot and lifetime after the FD walk.
		again := make([]byte, len(buf))
		againSize := C.size_t(len(again))
		if code := C.missing_args(C.int(pid), (*C.char)(unsafe.Pointer(&again[0])), &againSize); code != 0 {
			return Identity{}, syscall.Errno(code)
		}
		if againSize != size || !bytes.Equal(buf[:int(size)], again[:int(againSize)]) {
			return Identity{}, fmt.Errorf("worker saved path or arguments changed")
		}
		if code := C.missing_info(C.int(pid), &after); code != 0 {
			return Identity{}, syscall.Errno(code)
		}
		if before.pbi_start_tvsec != after.pbi_start_tvsec || before.pbi_start_tvusec != after.pbi_start_tvusec || before.pbi_uid != after.pbi_uid || uint32(after.pbi_uid) != uint32(os.Getuid()) {
			return Identity{}, fmt.Errorf("worker lifetime or user changed")
		}
	}
	if _, err := os.Lstat(string(raw[:n])); !errors.Is(err, os.ErrNotExist) {
		return Identity{}, fmt.Errorf("unresolved plausible writer: saved launch path still exists or is inaccessible (possible alias or replacement)")
	}
	// Do not apply this evidence after an exec has made normal inspection work.
	_, inspectErr := Inspect(pid)
	if err := missingExecutableStateError(inspectErr); err != nil {
		return Identity{}, err
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
	owns, socketErr := holdsListenerInspected(evidence, endpoint, missingExecutableEvidence)
	if socketErr != nil {
		return Identity{}, missingListenerError(pid, socketErr, missingIdentityError)
	}
	if owns {
		return Identity{}, fmt.Errorf("upgrade blocker pid=%d: missing executable %s owns target listener; unresolved plausible writer", pid, evidence.Executable)
	}
	// Neither a bounded known writer nor a target listener. No signal authority.
	return Identity{}, nil
}

// A successful inspection invalidates missing-path evidence; a real error must
// retain its identity so confirmed disappearance remains skippable.
func missingExecutableStateError(err error) error {
	if errors.Is(err, syscall.ENOENT) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("candidate executable state changed: inspection succeeded")
	}
	return fmt.Errorf("candidate executable state changed: %w", err)
}

// Descriptor errors (even ESRCH) do not prove process death. Only a fresh BSD
// identity query may authorize the discovery loop to skip this candidate.
func missingListenerError(pid int, socketErr error, probe func(int) error) error {
	identityErr := probe(pid)
	if errors.Is(identityErr, syscall.ESRCH) {
		return fmt.Errorf("upgrade candidate pid=%d disappeared after listener inspection: %w", pid, identityErr)
	}
	// Deliberately do not wrap socketErr: its ESRCH may describe an empty or
	// unstable FD inventory, not a missing process.
	return fmt.Errorf("upgrade blocker pid=%d: missing executable; listener unresolved: %v; fresh kernel identity: %v", pid, socketErr, identityErr)
}

func missingIdentityError(pid int) error {
	var info C.struct_proc_bsdinfo
	if code := C.missing_info(C.int(pid), &info); code != 0 {
		return syscall.Errno(code)
	}
	return nil
}

// Deliberately bounded to the observed production invocation. Other forms stay
// blocking rather than guessing about flags or future roles. argv[0] is ignored.
func missingWorkerSocket(raw []byte) string {
	if len(raw) < 5 {
		return ""
	}
	argc := int(binary.NativeEndian.Uint32(raw[:4]))
	if argc != 4 {
		return ""
	}
	data := raw[4:]
	n := bytes.IndexByte(data, 0)
	if n < 1 {
		return ""
	}
	data = data[n+1:]
	data = bytes.TrimLeft(data, "\x00")
	args := make([]string, 0, argc)
	for i := 0; i < argc; i++ {
		n = bytes.IndexByte(data, 0)
		if n < 0 {
			return ""
		}
		args = append(args, string(data[:n]))
		data = data[n+1:]
	}
	if args[1] != "worker" || args[2] != "--socket" || !filepath.IsAbs(args[3]) || len(args[3]) >= 104 {
		return ""
	}
	return args[3]
}

func missingWorkerSocketResult(matched, listener bool) error {
	if !matched || listener {
		return fmt.Errorf("unresolved plausible writer: worker socket absent or Internet listener present")
	}
	return nil
}
