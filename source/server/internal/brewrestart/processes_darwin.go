//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <string.h>
#include <sys/proc.h>

// cercano_copy_name copies the kernel's process name into name (at least
// 2*MAXCOMLEN+1 bytes): pbi_name, or the shorter pbi_comm when it is empty.
// Neither is guaranteed to be NUL-terminated at full length.
static void cercano_copy_name(const struct proc_bsdinfo *info, char *name) {
 const char *src = info->pbi_name[0] ? info->pbi_name : info->pbi_comm;
 size_t max = info->pbi_name[0] ? sizeof(info->pbi_name) : sizeof(info->pbi_comm);
 size_t n = strnlen(src, max);
 memcpy(name, src, n);
 name[n] = 0;
}

// cercano_process reads a process's parent, name, start time and executable
// path. A zombie or a process that exits meanwhile reports ESRCH. *gone is
// set when the process runs but the kernel gives no path: proc_pidpath
// fails with ENOENT once the executable or its directory was removed.
static int cercano_process(int pid, int *ppid, char *path, char *name,
                           long long *sec, long long *usec, int *gone) {
 struct proc_bsdinfo info;
 memset(&info, 0, sizeof(info));
 path[0] = 0;
 name[0] = 0;
 *gone = 0;
 errno = 0;
 if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, sizeof(info)) != sizeof(info))
  return errno ? errno : ESRCH;
 if (info.pbi_status == SZOMB) return ESRCH;
 *ppid = (int)info.pbi_ppid;
 *sec = (long long)info.pbi_start_tvsec;
 *usec = (long long)info.pbi_start_tvusec;
 cercano_copy_name(&info, name);
 errno = 0;
 if (proc_pidpath(pid, path, PROC_PIDPATHINFO_MAXSIZE) <= 0) {
  int e = errno;
  path[0] = 0;
  if (e == ESRCH) return ESRCH;
  *gone = 1;
 }
 return 0;
}
*/
import "C"

import "time"

// ListUserProcesses lists the current user's processes with their parent,
// executable, name and start time. For those details accepts it also reads
// the arguments and HOME. A process whose executable path the kernel can no
// longer give is listed with ExecutableGone and its name. Processes that exit
// while being read, and zombies, are skipped.
func ListUserProcesses(details func(UserProcess) bool) ([]UserProcess, error) {
	pids, err := ListCandidates()
	if err != nil {
		return nil, err
	}
	out := make([]UserProcess, 0, len(pids))
	for _, pid := range pids {
		var ppid, gone C.int
		var sec, usec C.longlong
		var path [C.PROC_PIDPATHINFO_MAXSIZE]C.char
		var name [2*C.MAXCOMLEN + 1]C.char
		if C.cercano_process(C.int(pid), &ppid, &path[0], &name[0], &sec, &usec, &gone) != 0 {
			continue
		}
		p := UserProcess{
			PID: pid, PPID: int(ppid),
			Executable:     C.GoString(&path[0]),
			ExecutableGone: gone != 0,
			Name:           C.GoString(&name[0]),
			Started:        time.Unix(int64(sec), int64(usec)*int64(time.Microsecond)),
		}
		if (p.Executable != "" || p.ExecutableGone) && details != nil && details(p) {
			if args, env, err := procArgsEnv(pid); err == nil {
				p.Args, p.Home, p.HomeKnown = args, envHome(env), true
				clear(env)
			}
		}
		out = append(out, p)
	}
	return out, nil
}
