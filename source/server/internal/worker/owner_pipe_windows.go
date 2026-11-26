package worker

import (
	"fmt"
	"os"
	"os/exec"
)

// Windows exec does not support ExtraFiles. Worker mode has no stdin protocol:
// dedicate its standard input handle to the lifetime pipe. Go's spawn path
// duplicates only the read end into the child; os.Pipe handles are non-inheritable.
func prepareOwnerPipe(cmd *exec.Cmd) (*os.File, *os.File, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdin = r
	cmd.Env = append(cmd.Environ(), ownerPipeEnv+"=stdin")
	return r, w, nil
}

func inheritedOwnerPipe(value string) (*os.File, error) {
	if value != "stdin" {
		return nil, fmt.Errorf("invalid owner pipe")
	}
	return os.Stdin, nil
}
