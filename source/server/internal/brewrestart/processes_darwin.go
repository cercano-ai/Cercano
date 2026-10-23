//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <string.h>

static int cercano_parent_path(int pid, int *ppid, char *path) {
 struct proc_bsdinfo info;
 memset(&info, 0, sizeof(info));
 path[0] = 0;
 errno = 0;
 if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, sizeof(info)) != sizeof(info))
  return errno ? errno : ESRCH;
 *ppid = (int)info.pbi_ppid;
 if (proc_pidpath(pid, path, PROC_PIDPATHINFO_MAXSIZE) <= 0) path[0] = 0;
 return 0;
}
*/
import "C"

// ListUserProcesses lists the current user's processes with their parent
// and executable. For those whose executable details accepts it also reads
// the arguments and HOME. Processes that exit while being read are skipped.
func ListUserProcesses(details func(executable string) bool) ([]UserProcess, error) {
	pids, err := ListCandidates()
	if err != nil {
		return nil, err
	}
	out := make([]UserProcess, 0, len(pids))
	for _, pid := range pids {
		var ppid C.int
		var path [C.PROC_PIDPATHINFO_MAXSIZE]C.char
		if C.cercano_parent_path(C.int(pid), &ppid, &path[0]) != 0 {
			continue
		}
		p := UserProcess{PID: pid, PPID: int(ppid), Executable: C.GoString(&path[0])}
		if p.Executable != "" && details != nil && details(p.Executable) {
			if args, env, err := procArgsEnv(pid); err == nil {
				p.Args, p.Home, p.HomeKnown = args, envHome(env), true
				clear(env)
			}
		}
		out = append(out, p)
	}
	return out, nil
}
