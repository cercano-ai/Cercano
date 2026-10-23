package stablelaunch

import (
	"bytes"
	"context"
"errors"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/enrollment"
	"cercano/source/server/internal/updatecoord/exclusion"
	"cercano/source/server/internal/updatecoord/selection"
	"cercano/source/server/internal/updatecoord/state"
	"cercano/source/server/pkg/installcontext"
	"cercano/source/server/pkg/installcontexttest"
)

const helperEnv = "CERCANO_STABLE_LAUNCH_FIXTURE"

type output struct {
	Args         []string
	Input, Cwd   string
	PID          int
	LockReleased bool
}

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) != "1" {
		os.Exit(m.Run())
	}
	loc, found, args, err := installcontext.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	client := os.Getenv("CERCANO_STABLE_ROLE") == "client"
	var verified installcontext.Verified
	if found {
		verified, err = installcontext.VerifyStartupVerified(context.Background(), loc, client)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
	}
	handled, code, err := Startup(args, client, verified)
	if handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(code)
	}
	if !found {
		fmt.Fprintln(os.Stderr, "fixture was not handed off")
		os.Exit(4)
	}
	root, err := installcontext.DefaultStateRoot()
	if err != nil {
		os.Exit(5)
	}
	store, err := state.OpenExistingReadOnly(context.Background(), root, loc.InstallID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(6)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	lock, err := exclusion.Acquire(ctx, store.Directory(), exclusion.Update)
	cancel()
	store.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "parent lock survived handoff", err)
		os.Exit(7)
	}
	lock.Close()
	if os.Getenv("CERCANO_STABLE_FD") == "3" {
		inherited := os.NewFile(3, "fixture handoff")
		data, e := io.ReadAll(inherited)
		if e != nil || string(data) != "private handoff" {
			fmt.Fprintln(os.Stderr, "lost inherited handoff", e)
			os.Exit(8)
		}
		inherited.Close()
	}
	data, _ := io.ReadAll(os.Stdin)
	cwd, _ := os.Getwd()
	_ = json.NewEncoder(os.Stdout).Encode(output{Args: args, Input: string(data), Cwd: cwd, PID: os.Getpid(), LockReleased: true})
	fmt.Fprintln(os.Stderr, "child stderr")
	os.Exit(17)
}

func fixture(t *testing.T, candidate ...bool) *installcontexttest.Install {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var f *installcontexttest.Install
	if len(candidate) > 0 && candidate[0] {
		f = installcontexttest.BuildCandidate(t, exe, exe)
	} else {
		f = installcontexttest.Build(t, exe, exe)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{f.AgentName, f.ClientName} {
		if err = os.WriteFile(filepath.Join(f.Root, name), data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestStableLauncherForegroundHandoff(t *testing.T) {
	for _, candidate := range []bool{false, true} {
		t.Run(fmt.Sprint("candidate=", candidate), func(t *testing.T) { testForegroundHandoff(t, candidate) })
	}
}

func testForegroundHandoff(t *testing.T, candidate bool) {
	f := fixture(t, candidate)
	for _, role := range []string{"agent", "client"} {
		t.Run(role, func(t *testing.T) {
			name := f.AgentName
			if role == "client" {
				name = f.ClientName
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			args := []string{"chat", "argument with spaces", "--", "--internal-install-id", "ordinary positional"}
			cmd := exec.CommandContext(ctx, filepath.Join(f.Root, name), args...)
			cmd.Env = append(f.ChildEnv(), helperEnv+"=1", "CERCANO_STABLE_ROLE="+role)
			cmd.Dir = f.Home
			cmd.Stdin = bytes.NewBufferString("terminal input\n")
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			pid := cmd.Process.Pid
			err := cmd.Wait()
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 17 {
				t.Fatalf("exit %v, stderr %s", err, &stderr)
			}
			var got output
			if err = json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err, out.String())
			}
			want, _ := json.Marshal(args)
			actual, _ := json.Marshal(got.Args)
			if !bytes.Equal(want, actual) || got.Input != "terminal input\n" || got.Cwd != f.Home || !got.LockReleased {
				t.Fatalf("handoff lost state: %+v", got)
			}
			if runtime.GOOS != "windows" && got.PID != pid {
				t.Fatal("Unix process identity changed")
			}
			if stderr.String() != "child stderr\n" {
				t.Fatalf("stderr %q", stderr.String())
			}
		})
	}
}

func TestStableLauncherRefusesUnsafeImageAndWrongRole(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	if _, err := prepare(ctx, filepath.Join(f.Root, f.AgentName), f.StateRoot, nil, true); err == nil {
		t.Fatal("wrong role accepted")
	}
	if err := os.Chmod(f.AgentExe, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := prepare(ctx, filepath.Join(f.Root, f.AgentName), f.StateRoot, nil, false); err == nil {
		t.Fatal("unsafe image accepted")
	}
}

func TestStableLauncherExcludesUpdateAndReleasesRefusal(t *testing.T) {
	f := fixture(t)
	store, err := state.OpenExistingReadOnly(context.Background(), f.StateRoot, f.InstallID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lock, err := exclusion.Acquire(context.Background(), store.Directory(), exclusion.Update)
	if err != nil {
		t.Fatal(err)
	}
 start := time.Now()
 if _, err = prepare(context.Background(), filepath.Join(f.Root, f.AgentName), f.StateRoot, nil, false); !errors.Is(err, installcontext.ErrLaunchBusy) { t.Fatalf("want retryable busy: %v", err) }
 if _, err = installcontext.VerifyChild(context.Background(), installcontext.Locators{InstallRoot:f.Root,InstallID:f.InstallID}, f.StateRoot, filepath.Join(f.Root, "versions", "v1", "bin", f.AgentName), false); !errors.Is(err, installcontext.ErrLaunchBusy) { t.Fatalf("child want retryable busy: %v", err) }
 if time.Since(start) > 2*time.Second { t.Fatal("launch exclusion not bounded") }
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	_, err = prepare(ctx, filepath.Join(f.Root, f.AgentName), f.StateRoot, nil, false)
	cancel()
	lock.Close()
	if err == nil {
		t.Fatal("launched across update lock")
	}
	p, err := prepare(context.Background(), filepath.Join(f.Root, f.AgentName), f.StateRoot, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	other, e := exclusion.Acquire(ctx, store.Directory(), exclusion.Update)
	cancel()
	if e == nil {
		other.Close()
		t.Fatal("prepared launch does not exclude update")
	}
	p.lock.Close()
	// Locator injection cannot dispatch or leave a lease held.
	if _, e = prepare(context.Background(), filepath.Join(f.Root, f.AgentName), f.StateRoot, f.LocatorArgs(), false); e == nil {
		t.Fatal("nested locators accepted")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	other, e = exclusion.Acquire(ctx, store.Directory(), exclusion.Update)
	if e != nil {
		t.Fatal(e)
	}
	other.Close()
}

func TestStableLauncherRefusesMissingOrCorruptSelectionAndEnrollment(t *testing.T) {
	for _, kind := range []string{"missing-selection", "corrupt-selection", "corrupt-enrollment"} {
		t.Run(kind, func(t *testing.T) {
			f := fixture(t)
			store, e := state.OpenExistingReadOnly(context.Background(), f.StateRoot, f.InstallID)
			if e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(store.Directory(), selection.FileName)
			store.Close()
			switch kind {
			case "missing-selection":
				e = os.Remove(path)
			case "corrupt-selection":
				e = os.WriteFile(path, []byte("{}"), 0600)
			case "corrupt-enrollment":
				e = os.WriteFile(filepath.Join(f.Root, enrollment.FileName), []byte("{}"), 0600)
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = prepare(context.Background(), filepath.Join(f.Root, f.AgentName), f.StateRoot, nil, false); e == nil {
				t.Fatal("unsafe startup accepted")
			}
		})
	}
}

func TestStableLauncherUnenrolledCopyDoesNotInitialize(t *testing.T) {
	f := fixture(t)
	if e := os.Remove(filepath.Join(f.Root, enrollment.FileName)); e != nil {
		t.Fatal(e)
	}
	// A missing state base must remain missing because enrollment is checked first.
	home := filepath.Join(t.TempDir(), "missing-home")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(f.Root, f.AgentName))
	env := []string{}
	for _, kv := range f.ChildEnv() {
		key := strings.ToUpper(strings.SplitN(kv, "=", 2)[0])
		if key != "HOME" && key != "USERPROFILE" && key != "LOCALAPPDATA" && key != "XDG_STATE_HOME" {
			env = append(env, kv)
		}
	}
	cmd.Env = append(env, helperEnv+"=1", "HOME="+home, "USERPROFILE="+home, "LOCALAPPDATA="+home, "XDG_STATE_HOME="+home)
	out, e := cmd.CombinedOutput()
	exit, ok := e.(*exec.ExitError)
	if !ok || exit.ExitCode() != 4 {
		t.Fatalf("unenrolled startup altered: %v %s", e, out)
	}
	if _, e = os.Stat(home); !os.IsNotExist(e) {
		t.Fatal("unenrolled startup created state", e)
	}
}

func TestBootWithoutDatabase(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt", "inaccessible"} {
		t.Run(mode, func(t *testing.T) {
			f := fixture(t)
			dir, err := installcontext.BootDirectory(f.StateRoot, f.InstallID)
			if err != nil {
				t.Fatal(err)
			}
			db := filepath.Join(dir, "state.db")
			if err = os.Remove(db); err != nil {
				t.Fatal(err)
			}
			if mode != "missing" {
				if err = os.WriteFile(db, []byte("not sqlite"), 0600); err != nil {
					t.Fatal(err)
				}
				if mode == "inaccessible" {
					if err = os.Chmod(db, 0000); err != nil {
						t.Fatal(err)
					}
					defer os.Chmod(db, 0600)
				}
			}
			for _, client := range []bool{false, true} {
				name := f.AgentName
				if client {
					name = f.ClientName
				}
				p, err := prepare(context.Background(), filepath.Join(f.Root, name), f.StateRoot, nil, client)
				if err != nil {
					t.Fatal(err)
				}
				p.lock.Close()
				_, err = installcontext.VerifyChild(context.Background(), installcontext.Locators{InstallRoot: f.Root, InstallID: f.InstallID}, f.StateRoot, p.target, client)
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
