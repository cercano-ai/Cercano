package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/brewrestart"
	pb "cercano/source/server/pkg/proto"
)

// Exercise the production adapter and real registry/launch-lock machinery while
// replacing only kernel process observation/control. No live hosts are touched.
func TestDevelopmentLegacyProductionAdapter(t *testing.T) {
	for _, mode := range []string{"accept", "decline", "stale-process", "stale-record", "stale-generation", "approval-rejected", "destination", "launch-busy", "held", "absence-failed", "use-failed", "cancelled", "final-native-refusal"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			state, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			hosts := filepath.Join(state, "hosts")
			if err = os.Mkdir(hosts, 0700); err != nil {
				t.Fatal(err)
			}
			source := &pb.LifecycleIdentity{Host: "A", Store: "store", Contract: "contract", Schema: 2}
			dest := &pb.LifecycleIdentity{Host: "B", Store: source.Store, Contract: source.Contract, Schema: 2}
			generation := filepath.Join(state, "generation-A")
			base := filepath.Join(hosts, fmt.Sprintf("%x", sha256.Sum256([]byte(generation))))
			record := launchSelection{Identity: source, Endpoint: "127.0.0.1:12345"}
			if err = launchExclusive(base+".json", record); err != nil {
				t.Fatal(err)
			}
			if err = launchExclusive(base+".starting", map[string]string{"host": "A", "generation": generation}); err != nil {
				t.Fatal(err)
			}
			// A separate host record is never part of the approved stop scope.
			sibling := filepath.Join(hosts, "unrelated.json")
			if err = launchExclusive(sibling, launchSelection{Identity: dest, Endpoint: "127.0.0.1:12347"}); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{base + ".json", base + ".starting", sibling} {
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if (mode == "stale-record" && path == base+".json") || (mode == "stale-generation" && path == base+".starting") {
						return
					}
					after, err := os.ReadFile(path)
					if err != nil || string(before) != string(after) {
						t.Errorf("unexpected registry mutation %s: %v", path, err)
					}
				})
			}
			lock, err := os.OpenFile(base+".lock", os.O_CREATE|os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			id := brewrestart.Identity{PID: 42, UID: uint32(os.Getuid()), StartSeconds: 100, Executable: filepath.Join(generation, "cercano")}
			adapter := newDevelopmentContainedSource(state, dest)
			// Transport authentication is exercised by TestLegacyShutdown; this fixture
			// isolates generation, process, consent and writer-exclusion checks.
			adapter.authenticate = func(context.Context, string, *pb.LifecycleIdentity) error { return nil }
			if adapter.inspect == nil || adapter.stop == nil || adapter.absent != nil {
				t.Fatal("production must use prepared-generation absence, not an override")
			}
			adapter.inspect = func(_ context.Context, host string, endpoint netip.AddrPort) (brewrestart.Identity, error) {
				if host != "A" || endpoint.String() != record.Endpoint {
					t.Fatalf("expanded source scope: %s %s", host, endpoint)
				}
				return id, nil
			}
			stops, approvals := 0, 0
			adapter.stop = func(ctx context.Context, host string, endpoint netip.AddrPort, expected brewrestart.Identity, validate func() error) error {
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("unbounded shutdown")
				}
				if err := devFlock(lock, devLockExclusive|devLockNonBlock); err == nil {
					t.Fatal("stop without launch exclusion")
				}
				if host != "A" || !expected.SameProcess(id) {
					t.Fatal("wrong stop target")
				}
				if err := validate(); err != nil {
					return err
				}
				stops++
				return nil
			}
			snapshot, err := adapter.Prepare(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "cancelled":
				cancel()
			case "decline":
				if stops != 0 {
					t.Fatal("stop on offer")
				}
				return
			case "stale-process":
				id.StartSeconds++
			case "stale-record":
				record.Endpoint = "127.0.0.1:12346"
				os.Remove(base + ".json")
				if err = launchExclusive(base+".json", record); err != nil {
					t.Fatal(err)
				}
			case "stale-generation":
				os.Remove(base + ".starting")
				if err = launchExclusive(base+".starting", map[string]string{"host": "A", "generation": generation + "-replacement"}); err != nil {
					t.Fatal(err)
				}
			case "destination":
				source = dest
			case "launch-busy":
				if err = devFlock(lock, devLockExclusive|devLockNonBlock); err != nil {
					t.Fatal(err)
				}
				defer devFlock(lock, devLockUnlock)
			}
			validate := func() error {
				approvals++
				if mode == "approval-rejected" {
					return fmt.Errorf("stale epoch")
				}
				return nil
			}
			uses, absences := 0, 0
			assertExcluded := func() {
				t.Helper()
				if err := devFlock(lock, devLockExclusive|devLockNonBlock); err == nil {
					t.Fatal("source can relaunch during held stop/absence/use")
				}
			}
			adapter.absent = func(_ context.Context, host string) error {
				absences++
				assertExcluded()
				if host != "A" {
					t.Fatal("wrong absence target")
				}
				if mode == "absence-failed" {
					return fmt.Errorf("source relaunched")
				}
				return nil
			}
			if mode == "held" || mode == "absence-failed" || mode == "use-failed" {
				err = adapter.stopAndUseSource(ctx, source, snapshot, validate, func(context.Context) error {
					uses++
					assertExcluded()
					if mode == "use-failed" {
						return fmt.Errorf("containment uncertain; no reassignment")
					}
					return nil
				})
				if stops != 1 || absences != 1 || (mode == "absence-failed" && uses != 0) || (mode != "absence-failed" && uses != 1) || (mode == "held") != (err == nil) {
					t.Fatalf("stops=%d absences=%d uses=%d err=%v", stops, absences, uses, err)
				}
			} else if mode == "final-native-refusal" {
				// The unit-test absence hook says absent, but the native final
				// verifier must reject this fixture's relative store path.
				bounded, done := context.WithTimeout(ctx, 75*time.Millisecond)
				defer done()
				err = adapter.WithStoppedSource(bounded, source, snapshot, validate, func(context.Context) error { uses++; return nil })
				if !errors.Is(err, context.DeadlineExceeded) || uses != 0 || stops != 1 || absences != 0 {
					t.Fatalf("native verification bypassed: stops=%d uses=%d absences=%d err=%v", stops, uses, absences, err)
				}
			} else {
				err = adapter.stopAndUseSource(ctx, source, snapshot, validate, func(context.Context) error {
					uses++
					assertExcluded()
					return nil
				})
			}
			if mode != "launch-busy" {
				if e := devFlock(lock, devLockExclusive|devLockNonBlock); e != nil {
					t.Fatalf("leaked launch exclusion: %v", e)
				}
				_ = devFlock(lock, devLockUnlock)
			}
			if mode == "held" || mode == "absence-failed" || mode == "use-failed" || mode == "final-native-refusal" {
				return
			}
			if mode == "accept" {
				if err != nil || stops != 1 || approvals != 1 {
					t.Fatalf("stop=%d approvals=%d err=%v", stops, approvals, err)
				}
			} else if err == nil || stops != 0 {
				t.Fatalf("unsafe stop=%d err=%v", stops, err)
			}
		})
	}
}

// Orchestration-only test seam. Production WithStoppedSource always supplies the
// native final verifier; this helper is not compiled into the host binary.
func (p *developmentContainedSource) stopAndUseSource(ctx context.Context, source *pb.LifecycleIdentity, snapshot string, validate func() error, use func(context.Context) error) error {
	return p.containSource(ctx, source, snapshot, validate, func(ctx context.Context, generation, store string) error {
		if err := p.preparedAbsent(ctx, source.Host, generation, store); err != nil {
			return err
		}
		return use(ctx)
	})
}
