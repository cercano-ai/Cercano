package brewrestart

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"testing"
	"time"
)

func TestCoordinateRestartOrderingAndFailures(t *testing.T) {
	for _, fail := range []string{"", "capture", "preflight", "lock", "changed", "shutdown", "wait-exit", "start", "ready", "absent"} {
		t.Run(fail, func(t *testing.T) {
			id := Identity{PID: 1, UID: 501, Executable: "/opt/homebrew/Cellar/cercano/1/bin/cercano", StartSeconds: 1}
			source := &fakeProcesses{pids: []int{1}, ids: map[int]Identity{1: id}, listeners: map[int]bool{1: true}}
			if fail == "absent" {
				source.pids = nil
			}
			var calls []string
			step := func(name string) error {
				calls = append(calls, name)
				if name == fail {
					return errors.New("injected failure")
				}
				return nil
			}
			ops := restartOps{
				source:    source,
				capture:   func(Identity) (LaunchState, error) { return LaunchState{Identity: id}, step("capture") },
				preflight: func(string, LaunchState) error { return step("preflight") },
				lock: func(context.Context, LaunchState) (func(), error) {
					if err := step("lock"); err != nil {
						return nil, err
					}
					if fail == "changed" {
						q := id
						q.StartSeconds++
						source.ids[1] = q
					}
					return func() { calls = append(calls, "release") }, nil
				},
				shutdown: func(context.Context, Identity, netip.AddrPort) error { return step("shutdown") },
				waitExit: func(context.Context, Identity) error { return step("wait-exit") },
				start:    func(string, LaunchState) (Identity, error) { return Identity{PID: 2}, step("start") },
				ready:    func(context.Context, Identity, netip.AddrPort) error { return step("ready") },
			}
			restarted, err := coordinateRestart(context.Background(), ops, "/opt/homebrew/Cellar/cercano/2/bin/cercano", 501, netip.MustParseAddrPort("127.0.0.1:12345"))
			wantErr := fail != "" && fail != "absent"
			if (err != nil) != wantErr || restarted != (fail == "") {
				t.Fatalf("restarted=%v err=%v", restarted, err)
			}
			all := []string{"capture", "preflight", "lock", "shutdown", "wait-exit", "start", "ready"}
			var want []string
			if fail != "absent" {
				for _, name := range all {
					if fail == "changed" && name == "shutdown" {
						break
					}
					want = append(want, name)
					if fail == name {
						break
					}
				}
				if fail != "capture" && fail != "preflight" && fail != "lock" {
					want = append(want, "release")
				}
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls %v want %v", calls, want)
			}
		})
	}
}

// readyOps is a restart whose every step succeeds and whose replacement is
// PID 7, with endpoint readiness forbidden.
func readyOps(t *testing.T) restartOps {
	id := Identity{PID: 1, UID: 501, Executable: "/opt/homebrew/Cellar/cercano/1/bin/cercano", StartSeconds: 1}
	source := &fakeProcesses{pids: []int{1}, ids: map[int]Identity{1: id}, listeners: map[int]bool{1: true}}
	return restartOps{
		source:    source,
		capture:   func(Identity) (LaunchState, error) { return LaunchState{Identity: id}, nil },
		preflight: func(string, LaunchState) error { return nil },
		lock:      func(context.Context, LaunchState) (func(), error) { return func() {}, nil },
		shutdown:  func(context.Context, Identity, netip.AddrPort) error { return nil },
		waitExit:  func(context.Context, Identity) error { return nil },
		start:     func(string, LaunchState) (Identity, error) { return Identity{PID: 7, StartSeconds: 70}, nil },
		ready: func(context.Context, Identity, netip.AddrPort) error {
			t.Error("endpoint readiness used")
			return nil
		},
	}
}

func runReadyOps(ops restartOps, timeout time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return coordinateRestart(ctx, ops, "/opt/homebrew/Cellar/cercano/2/bin/cercano", 501, netip.MustParseAddrPort("127.0.0.1:12345"))
}

// withReady confirms the replacement by probing it by process ID rather than
// through the old endpoint, re-probing until it is ready.
func TestCoordinateRestartWithReadyByPID(t *testing.T) {
	var probes int
	var alive []Identity
	ops := withReady(readyOps(t),
		func(id Identity) error { alive = append(alive, id); return nil },
		func(_ context.Context, pid int) bool {
			if pid != 7 {
				t.Errorf("probe got PID %d, want the replacement's 7", pid)
			}
			probes++
			return probes == 3
		})
	restarted, err := runReadyOps(ops, 5*time.Second)
	if err != nil || !restarted {
		t.Fatalf("restarted=%v err=%v", restarted, err)
	}
	if probes != 3 || len(alive) < 3 {
		t.Fatalf("probes=%d liveness checks=%d, want 3 and at least 3", probes, len(alive))
	}
	if alive[0].PID != 7 || alive[0].StartSeconds != 70 {
		t.Fatalf("liveness checked %+v, want the replacement's full identity", alive[0])
	}
}

// A replacement that exits during startup fails the restart at once rather
// than at the restart deadline.
func TestCoordinateRestartWithReadyFailsFastWhenReplacementExits(t *testing.T) {
	checks := 0
	ops := withReady(readyOps(t),
		func(Identity) error {
			checks++
			if checks >= 2 {
				return errReplacementExited
			}
			return nil
		},
		func(context.Context, int) bool { return false })
	start := time.Now()
	restarted, err := runReadyOps(ops, 10*time.Second)
	if restarted || !errors.Is(err, errReplacementExited) {
		t.Fatalf("restarted=%v err=%v, want errReplacementExited", restarted, err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("failure took %s, want it well before the 10s deadline", elapsed)
	}
}

// A replacement that never registers fails at the restart deadline.
func TestCoordinateRestartWithReadyHonorsDeadline(t *testing.T) {
	ops := withReady(readyOps(t), func(Identity) error { return nil }, func(context.Context, int) bool { return false })
	restarted, err := runReadyOps(ops, 300*time.Millisecond)
	if restarted || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("restarted=%v err=%v, want the deadline", restarted, err)
	}
}
