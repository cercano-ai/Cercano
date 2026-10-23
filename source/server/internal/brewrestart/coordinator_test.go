package brewrestart

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"testing"
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

// withReady confirms the replacement by its process ID rather than by the old
// endpoint, and a readiness failure still fails the restart.
func TestCoordinateRestartWithReadyByPID(t *testing.T) {
	for _, readyErr := range []error{nil, errors.New("not registered")} {
		id := Identity{PID: 1, UID: 501, Executable: "/opt/homebrew/Cellar/cercano/1/bin/cercano", StartSeconds: 1}
		source := &fakeProcesses{pids: []int{1}, ids: map[int]Identity{1: id}, listeners: map[int]bool{1: true}}
		ops := restartOps{
			source:    source,
			capture:   func(Identity) (LaunchState, error) { return LaunchState{Identity: id}, nil },
			preflight: func(string, LaunchState) error { return nil },
			lock:      func(context.Context, LaunchState) (func(), error) { return func() {}, nil },
			shutdown:  func(context.Context, Identity, netip.AddrPort) error { return nil },
			waitExit:  func(context.Context, Identity) error { return nil },
			start:     func(string, LaunchState) (Identity, error) { return Identity{PID: 7}, nil },
			ready: func(context.Context, Identity, netip.AddrPort) error {
				t.Error("endpoint readiness used")
				return nil
			},
		}
		var gotPID int
		ops = withReady(ops, func(_ context.Context, pid int) error { gotPID = pid; return readyErr })
		restarted, err := coordinateRestart(context.Background(), ops, "/opt/homebrew/Cellar/cercano/2/bin/cercano", 501, netip.MustParseAddrPort("127.0.0.1:12345"))
		if gotPID != 7 {
			t.Fatalf("ready got PID %d, want the replacement's 7", gotPID)
		}
		if restarted != (readyErr == nil) || (err != nil) != (readyErr != nil) {
			t.Fatalf("readyErr=%v: restarted=%v err=%v", readyErr, restarted, err)
		}
	}
}
