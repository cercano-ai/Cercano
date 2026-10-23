package localruntime

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/nativeprocess"
)

func TestOwnershipHelper(t *testing.T) {
	if os.Getenv("CERCANO_OWNERSHIP_HELPER") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}
func ownedHelper(t *testing.T, o *ProcessOwner) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnershipHelper$")
	cmd.Env = append(os.Environ(), "CERCANO_OWNERSHIP_HELPER=1")
	if err := o.StartOwned(cmd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { o.RetireOwned(cmd); _ = cmd.Process.Kill(); _ = cmd.Wait(); o.ExitedOwned(cmd) })
	return cmd
}
func TestManagedInitialLateReplacement(t *testing.T) {
	var o ProcessOwner
	first := ownedHelper(t, &o)
	seen := make(chan int, 4)
	ctx, cancel, err := o.WatchOwnedProcesses(context.Background(), func(_ context.Context, id nativeprocess.Identity) error { seen <- id.PID; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if pid := <-seen; pid != first.Process.Pid {
		t.Fatal(pid)
	}
	second := ownedHelper(t, &o)
	select {
	case pid := <-seen:
		if pid != second.Process.Pid {
			t.Fatal(pid)
		}
	case <-time.After(time.Second):
		t.Fatal("late birth missing")
	}
	o.RetireOwned(first)
	_ = first.Process.Kill()
	_ = first.Wait()
	o.ExitedOwned(first)
	replacement := ownedHelper(t, &o)
	select {
	case pid := <-seen:
		if pid != replacement.Process.Pid {
			t.Fatal(pid)
		}
	case <-time.After(time.Second):
		t.Fatal("replacement missing")
	}
	if ctx.Err() != nil {
		t.Fatal(context.Cause(ctx))
	}
}
func TestManagedBoundedCancellation(t *testing.T) {
	var o ProcessOwner
	cmd := ownedHelper(t, &o)
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	result := make(chan error, 1)
	go func() {
		_, cancel, err := o.WatchOwnedProcesses(context.Background(), func(context.Context, nativeprocess.Identity) error { close(entered); <-release; return nil })
		if cancel != nil {
			cancel()
		}
		result <- err
	}()
	<-entered
	start := time.Now()
	o.RetireOwned(cmd)
	if time.Since(start) > time.Second {
		t.Fatal("retirement blocked")
	}
	if err := <-result; err == nil {
		t.Fatal("timeout accepted")
	}
	// A failed stop is not an exit, regardless of dashboard state.
	if _, _, err := o.WatchOwnedProcesses(context.Background(), func(context.Context, nativeprocess.Identity) error { return nil }); err == nil {
		t.Fatal("retiring command disappeared")
	}
}
func TestManagedNoObserverAndAdopted(t *testing.T) {
	var o ProcessOwner
	cmd := ownedHelper(t, &o)
	start := time.Now()
	o.RetireOwned(cmd)
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("unobserved retirement delayed")
	}
	var adopted ProcessOwner
	adopted.UncertainOwnership()
	if _, _, err := adopted.WatchOwnedProcesses(context.Background(), func(context.Context, nativeprocess.Identity) error { return nil }); err == nil {
		t.Fatal("adopted coverage accepted")
	}
}
func TestManagedCancellationAndEarlyExit(t *testing.T) {
	for _, early := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "exit"}[early], func(t *testing.T) {
			var o ProcessOwner
			cmd := ownedHelper(t, &o)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered := make(chan struct{})
			result := make(chan error, 1)
			go func() {
				_, stop, err := o.WatchOwnedProcesses(parent, func(ctx context.Context, _ nativeprocess.Identity) error {
					close(entered)
					<-ctx.Done()
					return ctx.Err()
				})
				if stop != nil {
					stop()
				}
				result <- err
			}()
			<-entered
			if early {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				o.ExitedOwned(cmd)
			} else {
				cancel()
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("invalid observation accepted")
				}
			case <-time.After(time.Second):
				t.Fatal("observation blocked")
			}
		})
	}
}

func TestManagedOwnersOptionalCoverage(t *testing.T) {
	m := &InMemoryManager{providers: map[string]Provider{"custom": &fakeProvider{}}}
	if _, err := m.ManagedOwners(); err == nil {
		t.Fatal("custom provider coverage accepted")
	}
	m.providers = map[string]Provider{}
	if owners, err := m.ManagedOwners(); err != nil || len(owners) != 0 {
		t.Fatalf("empty ownership: %v %v", owners, err)
	}
}
