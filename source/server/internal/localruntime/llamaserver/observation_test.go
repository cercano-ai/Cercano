package llamaserver

import (
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/updatecoord/nativeprocess"
	"context"
	"os"
	"testing"
	"time"
)

// The test executable ignores model CLI arguments in this inert child mode.
func init() {
	if os.Getenv("CERCANO_PROVIDER_OWNER_HELPER") == "1" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
}
func TestManagedProviderBirthAndReplacement(t *testing.T) {
	t.Setenv("CERCANO_PROVIDER_OWNER_HELPER", "1")
	p := &Provider{running: make(map[string]*managedInstance)}
	inst := &managedInstance{record: localruntime.InstanceRecord{ID: "test"}}
	p.totalRAM = func() int64 { return 128 << 30 }
	p.nonEvictable = func() (int64, bool) { return 0, true }
	inst.plannedContext.Tokens = 1024
	p.running["test"] = inst
	if err := p.startProcess("test", os.Args[0], nil); err != nil {
		t.Fatal(err)
	}
	first := inst.cmd
	t.Cleanup(func() { _ = first.Process.Kill(); _ = first.Wait(); p.ExitedOwned(first) })
	seen := make(chan int, 4)
	observed, cancel, err := p.WatchOwnedProcesses(context.Background(), func(_ context.Context, id nativeprocess.Identity) error { seen <- id.PID; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if pid := <-seen; pid != first.Process.Pid {
		t.Fatal(pid)
	}
	p.RetireOwned(first)
	_ = first.Process.Kill()
	_ = first.Wait()
	p.ExitedOwned(first)
	// The supervisor's replacement path is the same startProcess call.
	if err := p.startProcess("test", os.Args[0], nil); err != nil {
		t.Fatal(err)
	}
	second := inst.cmd
	t.Cleanup(func() { _ = second.Process.Kill(); _ = second.Wait(); p.ExitedOwned(second) })
	select {
	case pid := <-seen:
		if pid != second.Process.Pid {
			t.Fatal(pid)
		}
	case <-time.After(time.Second):
		t.Fatal("replacement not captured")
	}
	if observed.Err() != nil {
		t.Fatal(context.Cause(observed))
	}
}
func TestManagedAdoptedStopPreservesProcess(t *testing.T) {
	t.Setenv("CERCANO_PROVIDER_OWNER_HELPER", "1")
	p := &Provider{running: make(map[string]*managedInstance)}
	inst := &managedInstance{record: localruntime.InstanceRecord{ID: "test"}}
	p.totalRAM = func() int64 { return 128 << 30 }
	p.nonEvictable = func() (int64, bool) { return 0, true }
	inst.plannedContext.Tokens = 1024
	p.running["test"] = inst
	if err := p.startProcess("test", os.Args[0], nil); err != nil {
		t.Fatal(err)
	}
	cmd := inst.cmd
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); p.ExitedOwned(cmd) }()
	// Simulate an external owner's record, deliberately with no protecting
	// sibling PID: lack of ownership evidence must never authorize a kill.
	external := &Provider{running: map[string]*managedInstance{"adopted": {adopted: true, record: localruntime.InstanceRecord{PID: cmd.Process.Pid}}}}
	external.UncertainOwnership()
	if err := external.Stop(context.Background(), "adopted"); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeprocess.Inspect(cmd.Process.Pid); err != nil {
		t.Fatalf("adopted process lost: %v", err)
	}
	if _, _, err := external.WatchOwnedProcesses(context.Background(), func(context.Context, nativeprocess.Identity) error { return nil }); err == nil {
		t.Fatal("adopted coverage accepted")
	}
}
