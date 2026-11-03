package main

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if _, err = guard(context.Background(), "A"); err == nil {
		t.Fatal("concurrent launch accepted")
	}
	if checks != 0 {
		t.Fatal("verification before launch exclusion")
	}
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	release, err := guard(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		t.Fatal("recovery did not retain launch exclusion")
	}
	release()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if _, err = guard(context.Background(), "unknown"); err == nil {
		t.Fatal("missing record accepted")
	}
	live := developmentRecoveryGuardWithVerifier(state, dest, func(context.Context, string) error { return fmt.Errorf("source still alive") })
	if _, err = live(context.Background(), "A"); err == nil {
		t.Fatal("live source accepted")
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal("failure leaked lock")
	}
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
}
