package main

import (
	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/internal/recoveryguard"
	pb "cercano/source/server/pkg/proto"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func developmentRecoveryGuard(state string, destination *pb.LifecycleIdentity) func(context.Context, string) (func(), error) {
	return developmentRecoveryGuardWithVerifier(state, destination, brewrestart.VerifyHostAbsent)
}

func developmentRecoveryGuardWithVerifier(state string, destination *pb.LifecycleIdentity, verify func(context.Context, string) error) func(context.Context, string) (func(), error) {
	return func(ctx context.Context, host string) (func(), error) {
		hosts := filepath.Join(state, "hosts")
		if err := launchPrivateDir(hosts); err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(hosts)
		if err != nil {
			return nil, err
		}
		record := ""
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			path := filepath.Join(hosts, entry.Name())
			var s launchSelection
			if err := launchRead(path, &s); err != nil {
				return nil, err
			}
			if s.Identity.GetHost() != host {
				continue
			}
			if record != "" {
				return nil, fmt.Errorf("duplicate source host records")
			}
			if s.Identity.GetStore() != destination.Store || s.Identity.GetContract() != destination.Contract || s.Identity.GetSchema() != destination.Schema {
				return nil, fmt.Errorf("incompatible recovery source")
			}
			record = path
		}
		if record == "" {
			return nil, fmt.Errorf("source has no verifiable host record")
		}
		base := strings.TrimSuffix(record, ".json")
		if err := launchPrivateFile(base + ".lock"); err != nil {
			return nil, err
		}
		lock, err := os.OpenFile(base+".lock", os.O_RDWR|devOpenNoFollow, 0)
		if err != nil {
			return nil, err
		}
		release := func() { _ = devFlock(lock, devLockUnlock); _ = lock.Close() }
		if err = devFlock(lock, devLockExclusive|devLockNonBlock); err != nil {
			lock.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
				return nil, fmt.Errorf("%w: launch exclusion busy", recoveryguard.ErrLiveSource)
			}
			return nil, err
		}
		success := false
		defer func() {
			if !success {
				release()
			}
		}()
		var intent map[string]string
		if err = launchRead(base+".starting", &intent); err != nil {
			return nil, err
		}
		if intent["host"] != host || !developmentInstanceMatches(base, intent) {
			return nil, fmt.Errorf("source start intent does not match exact host record")
		}
		var again launchSelection
		if err = launchRead(record, &again); err != nil {
			return nil, err
		}
		if again.Identity.GetHost() != host {
			return nil, fmt.Errorf("source record changed")
		}
		if err = verify(ctx, host); err != nil {
			return nil, err
		}
		success = true
		return release, nil
	}
}
