package main

import (
	"cercano/source/server/internal/recoveryguard"
	pb "cercano/source/server/pkg/proto"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentRecoveryRegistryExclusion(t *testing.T) {
	state, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hosts := filepath.Join(state, "hosts")
	if err = os.Mkdir(hosts, 0700); err != nil {
		t.Fatal(err)
	}
	generation := filepath.Join(state, "generation")
	base := filepath.Join(hosts, fmt.Sprintf("%x", sha256.Sum256([]byte(generation))))
	source := &pb.LifecycleIdentity{Host: "A", Build: "old", Store: filepath.Join(state, "conversations.db"), Contract: "test", Schema: 2}
	dest := &pb.LifecycleIdentity{Host: "B", Build: "new", Store: source.Store, Contract: source.Contract, Schema: source.Schema}
	if err = launchExclusive(base+".json", launchSelection{Identity: source}); err != nil {
		t.Fatal(err)
	}
	if err = launchExclusive(base+".starting", map[string]string{"host": "A", "generation": generation}); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(base+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	checks := 0
	guard := developmentRecoveryGuardWithVerifier(state, dest, func(_ context.Context, host string) error {
		checks++
		if host != "A" {
			t.Fatal(host)
		}
		return nil
	})
	if err = devFlock(lock, devLockExclusive|devLockNonBlock); err != nil {
		t.Fatal(err)
	}
	if _, err = guard(context.Background(), "A"); !errors.Is(err, recoveryguard.ErrLiveSource) {
		t.Fatal("concurrent launch accepted")
	}
	if checks != 0 {
		t.Fatal("verification before launch exclusion")
	}
	devFlock(lock, devLockUnlock)
	release, err := guard(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}
	if err = devFlock(lock, devLockExclusive|devLockNonBlock); err == nil {
		t.Fatal("recovery did not retain launch exclusion")
	}
	release()
	if err = devFlock(lock, devLockExclusive|devLockNonBlock); err != nil {
		t.Fatal(err)
	}
	devFlock(lock, devLockUnlock)
	if _, err = guard(context.Background(), "unknown"); err == nil || errors.Is(err, recoveryguard.ErrLiveSource) {
		t.Fatal("missing record accepted")
	}
	live := developmentRecoveryGuardWithVerifier(state, dest, func(context.Context, string) error { return fmt.Errorf("source still alive") })
	if _, err = live(context.Background(), "A"); err == nil || errors.Is(err, recoveryguard.ErrLiveSource) {
		t.Fatal("live source accepted")
	}
	if err = devFlock(lock, devLockExclusive|devLockNonBlock); err != nil {
		t.Fatal("failure leaked lock")
	}
	devFlock(lock, devLockUnlock)
}

func TestDevelopmentRecoveryGuardErrorsRemainUnclassified(t *testing.T) {
	for _, mode := range []string{"metadata", "identity", "duplicate", "inspection", "live"} {
		t.Run(mode, func(t *testing.T) {
			state, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			hosts := filepath.Join(state, "hosts")
			if err = os.Mkdir(hosts, 0700); err != nil {
				t.Fatal(err)
			}
			generation := filepath.Join(state, "generation")
			base := filepath.Join(hosts, fmt.Sprintf("%x", sha256.Sum256([]byte(generation))))
			source := &pb.LifecycleIdentity{Host: "A", Store: filepath.Join(state, "store"), Contract: "test", Schema: 2}
			dest := &pb.LifecycleIdentity{Host: "B", Store: source.Store, Contract: source.Contract, Schema: source.Schema}
			if err = launchExclusive(base+".json", launchSelection{Identity: source}); err != nil {
				t.Fatal(err)
			}
			if err = launchExclusive(base+".starting", map[string]string{"host": "A", "generation": generation}); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(base+".lock", nil, 0600); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "metadata":
				if err = os.WriteFile(base+".json", []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "identity":
				dest.Contract = "incompatible"
			case "duplicate":
				if err = launchExclusive(filepath.Join(hosts, "duplicate.json"), launchSelection{Identity: source}); err != nil {
					t.Fatal(err)
				}
			}
			guard := developmentRecoveryGuardWithVerifier(state, dest, func(context.Context, string) error {
				if mode == "live" {
					return recoveryguard.ErrLiveSource
				}
				return fmt.Errorf("inspection failed")
			})
			release, err := guard(context.Background(), "A")
			if release != nil {
				release()
				t.Fatal("failed guard returned exclusion")
			}
			if err == nil || errors.Is(err, recoveryguard.ErrLiveSource) != (mode == "live") {
				t.Fatal("incorrect guard classification")
			}
		})
	}
}
