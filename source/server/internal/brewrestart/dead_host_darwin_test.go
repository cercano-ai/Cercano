//go:build darwin && cgo

package brewrestart

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
)

func TestRecoveryProcessHelper(t *testing.T) {
	if os.Getenv("CERCANO_RECOVERY_HELPER") != "1" {
		return
	}
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}
func TestDeadHostKernelVerification(t *testing.T) {
	host := "disposable-host-absent-test"
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecoveryProcessHelper$", "--", "--host", host)
	cmd.Env = append(os.Environ(), "CERCANO_RECOVERY_HELPER=1")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		in.Close()
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("helper %q %v", line, err)
	}
	list := func() ([]int, error) { return []int{cmd.Process.Pid}, nil }
	if err = verifyHostAbsent(context.Background(), host, list); err == nil {
		t.Fatal("live host accepted as dead")
	}
	if err = verifyHostAbsent(context.Background(), "different-host", list); err != nil {
		t.Fatal(err)
	}
	in.Close()
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err = verifyHostAbsent(context.Background(), host, list); err != nil {
		t.Fatal(err)
	}
	if err = verifyHostAbsent(context.Background(), host, func() ([]int, error) { return nil, fmt.Errorf("incomplete inventory") }); err == nil {
		t.Fatal("incomplete inventory accepted")
	}
}
