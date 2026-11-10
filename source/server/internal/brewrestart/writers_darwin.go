//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <stdio.h>
// Read-only observation. Path uncertainty is refusal, never signal authority.
static int recovery_open_store(int pid, const char *store, int *found) {
 struct stat target[4]; int valid[4];
 const char *suffix[4]={"","-wal","-shm","-journal"};
 for(int i=0;i<4;i++){char path[4096];if(snprintf(path,sizeof(path),"%s%s",store,suffix[i])>=sizeof(path))return EOVERFLOW;errno=0;valid[i]=stat(path,&target[i])==0;if(!valid[i] && errno!=ENOENT)return errno;}
 errno=0;
 int n=proc_pidinfo(pid,PROC_PIDLISTFDS,0,NULL,0);
 if(n<=0) return errno ? errno : EAGAIN;
 int capacity=n+32*sizeof(struct proc_fdinfo);
 if(capacity>16*1024*1024) return EOVERFLOW;
 struct proc_fdinfo *fds=calloc(1,capacity);
 if(!fds) return ENOMEM;
 n=proc_pidinfo(pid,PROC_PIDLISTFDS,0,fds,capacity);
 if(n<=0 || n>=capacity || n%sizeof(*fds)) {free(fds);return EAGAIN;}
 *found=0;
 for(int i=0;i<n/sizeof(*fds);i++) {
  if(fds[i].proc_fdtype!=PROX_FDTYPE_VNODE) continue;
  struct vnode_fdinfowithpath v; memset(&v,0,sizeof(v)); errno=0;
  if(proc_pidfdinfo(pid,fds[i].proc_fd,PROC_PIDFDVNODEPATHINFO,&v,sizeof(v))!=sizeof(v)) {
   int e=errno ? errno : EAGAIN;free(fds);return e;
  }
  for(int j=0;j<4;j++)if(valid[j] && v.pvip.vip_vi.vi_stat.vst_dev==target[j].st_dev && v.pvip.vip_vi.vi_stat.vst_ino==target[j].st_ino)*found=1;
  const char *p=v.pvip.vip_path;
  if(!*p) {free(fds);return EAGAIN;}
  size_t len=strlen(store);
  if(!strncmp(p,store,len) && (!p[len] || !strcmp(p+len,"-wal") || !strcmp(p+len,"-shm") || !strcmp(p+len,"-journal"))) *found=1;
 }
 free(fds);return 0;
}
*/
import "C"
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// VerifyPreparedWritersAbsent is deliberately conservative: every process from
// the source generation blocks, including unrelated same-generation hosts. It
// never stops them. The destination is the caller and is the only store holder
// exempted. This observation is meaningful only after authenticated source death
// and under continuous source launch exclusion; it is NOT an arbitrary SQL fence.
func VerifyPreparedWritersAbsent(ctx context.Context, generation, store string) error {
	if !filepath.IsAbs(generation) || !filepath.IsAbs(store) {
		return fmt.Errorf("absolute prepared generation and store required")
	}
	pids, err := ListCandidates()
	if err != nil {
		return err
	}
	for _, pid := range pids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if pid == os.Getpid() {
			continue
		}
		executable, _, err := recoveryProcessArguments(pid)
		if err == syscall.ESRCH {
			continue
		}
		if err != nil {
			return fmt.Errorf("writer inventory pid=%d: %w", pid, err)
		}
		if executable == filepath.Join(generation, "cercano") {
			return fmt.Errorf("source generation process still present pid=%d", pid)
		}
		found, code := preparedStoreDescriptors(pid, store)
		if code != 0 {
			// Only kernel-confirmed death permits a vanished descriptor inventory.
			if _, _, gone := recoveryProcessArguments(pid); gone == syscall.ESRCH {
				continue
			}
			return fmt.Errorf("store descriptor inventory uncertain pid=%d: %w", pid, code)
		}
		if found {
			return fmt.Errorf("store remains open in pid=%d; external actions may be uncertain and must not be replayed", pid)
		}
	}
	return nil
}

// Kept separate so descriptor matching is exercised against fixture-owned files,
// without treating an injected empty process inventory as shipping evidence.
func preparedStoreDescriptors(pid int, store string) (bool, syscall.Errno) {
	name := C.CString(store)
	defer C.free(unsafe.Pointer(name))
	var found C.int
	code := syscall.Errno(C.recovery_open_store(C.int(pid), name, &found))
	return found != 0, code
}
