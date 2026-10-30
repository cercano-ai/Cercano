//go:build unix

package agentclient

import (
	"bufio"
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Run the real Python main/lock and the SDK lock, replacing only host startup.
func TestPythonLauncherSDKLockThroughReadiness(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	launcher, err := filepath.Abs("../../scripts/cercano-launcher.py")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	release, err := AcquireAutoLaunchLock(context.Background(), tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	script := `import importlib.util,os,sys
os.environ['TMPDIR']=sys.argv[2]
s=importlib.util.spec_from_file_location('launcher',sys.argv[1]); m=importlib.util.module_from_spec(s); s.loader.exec_module(m)
sys.argv=['launcher','unused','unused']
def startup():
 print('starting',flush=True)
 sys.stdin.readline()
 print('ready',flush=True)
 sys.stdin.readline()
m.launch_main=startup
print('waiting',flush=True)
m.main()
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c", script, launcher, tmp)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { stdin.Close(); cmd.Process.Kill(); cmd.Wait() }()
	lines := make(chan string, 4)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	expect := func(want string) {
		t.Helper()
		select {
		case got := <-lines:
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	expect("waiting")
	select {
	case line := <-lines:
		t.Fatalf("Python bypassed SDK lock: %s", line)
	case <-time.After(150 * time.Millisecond):
	}
	release()
	expect("starting")
	blocked := func() {
		t.Helper()
		c, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		defer cancel()
		unlock, err := AcquireAutoLaunchLock(c, tmp)
		if err == nil {
			unlock()
			t.Fatal("SDK bypassed Python startup lock")
		}
	}
	blocked()
	stdin.Write([]byte("ready\n"))
	expect("ready")
	blocked()
	stdin.Write([]byte("exit\n"))
	unlock, err := AcquireAutoLaunchLock(ctx, tmp)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}
