//go:build darwin || linux

package worker

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
)

// Exec a real descendant after consuming the inherited descriptor, verifying
// neither the advertised capability nor the pipe itself crosses the exec.
func TestOwnerPipeDescendant(t *testing.T) {
	if fdText := os.Getenv("CERCANO_OWNER_DESCENDANT_FD"); fdText != "" {
		if os.Getenv(ownerPipeEnv) != "" {
			t.Fatal("owner capability advertised to descendant")
		}
		fd, _ := strconv.Atoi(fdText)
		var st syscall.Stat_t
		if syscall.Fstat(fd, &st) == nil && strconv.FormatUint(uint64(st.Ino), 10) == os.Getenv("CERCANO_OWNER_DESCENDANT_INO") {
			t.Fatal("owner pipe inherited by descendant")
		}
		return
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	// ExtraFiles arrives without CLOEXEC. Reproduce that inherited state.
	fd, err := syscall.Dup(int(r.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	f, err := inheritedOwnerPipe(strconv.Itoa(fd))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnerPipeDescendant$")
	cmd.Env = append(os.Environ(), ownerPipeEnv+"=", "CERCANO_OWNER_DESCENDANT_FD="+strconv.Itoa(fd), "CERCANO_OWNER_DESCENDANT_INO="+strconv.FormatUint(uint64(st.Ino), 10))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("descendant: %v\n%s", err, output)
	}
}
