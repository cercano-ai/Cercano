package worker

import (
	"os"
	"slices"
	"testing"
)

func TestWorkerInheritsProfile(t *testing.T) {
	t.Setenv("CERCANO_PROFILE", "work")
	cmd := newWorkerCommand("/bin/cercano", "/tmp/w.sock")
	env := cmd.Env
	if env == nil { // nil Env means the child inherits the host environment
		env = os.Environ()
	}
	if !slices.Contains(env, "CERCANO_PROFILE=work") {
		t.Fatal("worker environment lacks CERCANO_PROFILE=work")
	}
}
