//go:build !windows

package worker

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func prepareOwnerPipe(cmd *exec.Cmd) (*os.File, *os.File, error) {
	r, w, err := os.Pipe() // both descriptors are close-on-exec
	if err != nil {
		return nil, nil, err
	}
	fd := 3 + len(cmd.ExtraFiles)
	cmd.ExtraFiles = append(cmd.ExtraFiles, r) // never pass w
	cmd.Env = append(cmd.Environ(), ownerPipeEnv+"="+strconv.Itoa(fd))
	return r, w, nil
}

func inheritedOwnerPipe(value string) (*os.File, error) {
	fd, err := strconv.Atoi(value)
	if err != nil || fd < 3 {
		return nil, fmt.Errorf("invalid owner descriptor")
	}
	syscall.CloseOnExec(fd) // descendants do not need even the read end
	f := os.NewFile(uintptr(fd), "worker-owner")
	if f == nil {
		return nil, fmt.Errorf("invalid owner pipe")
	}
	return f, nil
}
