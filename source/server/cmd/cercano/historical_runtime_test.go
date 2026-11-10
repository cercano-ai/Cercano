package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"

	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/pkg/agentclient"
	pb "cercano/source/server/pkg/proto"
)

// Runtime evidence for the pinned fixture graph only, not a production
// authorization certificate for arbitrary old binaries or external effects.
func historicalRuntimeContainment(t *testing.T, ctx context.Context, state, runtime, binary string, env []string, source launchSelection, unblock func(), pending <-chan agentclient.StreamMsg, owned func() ([]nativeControllerProcess, error)) {
	t.Helper()
	destination := &pb.LifecycleIdentity{Host: "fixture-destination", Store: source.Identity.Store, Contract: source.Identity.Contract, Schema: source.Identity.Schema}
	adapter := newDevelopmentLegacyStop(state, destination)
	snapshot, err := adapter.Prepare(ctx, source.Identity)
	if err != nil {
		t.Fatal(err)
	}
	blocked := strings.HasSuffix(t.Name(), "blocked-timeout")
	duration := 5 * time.Second
	if blocked {
		duration = 150 * time.Millisecond
	} else {
		time.AfterFunc(100*time.Millisecond, unblock)
	}
	stopCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	used := false
	verify := func(ctx context.Context) error {
		used = true
		processes, err := owned()
		if err != nil {
			return err
		}
		if len(processes) != 0 {
			return fmt.Errorf("owned host/worker still present under exclusion: %+v", processes)
		}
		t.Log("owned host and worker absent under retained exclusion; host-owned task store writer terminated")
		launchCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		contender := exec.CommandContext(launchCtx, binary, "development-launch", "--generation", filepath.Dir(binary), "--state", state, "--runtime-root", runtime)
		contender.Env = env
		output, err := contender.CombinedOutput()
		if launchCtx.Err() == nil {
			return fmt.Errorf("historical launch escaped exclusion: %v %s", err, output)
		}
		// Check all coexistence-store files, including WAL. This observation follows
		// actual in-flight worker callbacks and supplements the pinned source trace.
		before, err := historicalStoreFiles(state)
		if err != nil {
			return err
		}
		select {
		case <-time.After(350 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
		after, err := historicalStoreFiles(state)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(before, after) {
			return fmt.Errorf("owned store changed after shutdown under exclusion")
		}
		t.Log("actual historical launcher contended; store/WAL unchanged after drain under exclusion")
		return nil
	}
	err = adapter.withStoppedSource(stopCtx, source.Identity, snapshot, func() error { return nil }, verify)
	if blocked {
		if !errors.Is(err, context.DeadlineExceeded) || used {
			t.Fatalf("blocked provider authorized recovery: used=%v err=%v", used, err)
		}
		t.Log("actual blocked worker turn timed out without recovery callback")
		unblock()
		// A timed-out attempt grants no recovery authority. Reacquire exclusion only
		// for observation/cleanup; never replay shutdown or change ownership.
		err = adapter.withSource(ctx, source.Identity, func(_ launchSelection, _ netip.AddrPort) error {
			for {
				if err := brewrestart.VerifyHostAbsent(ctx, source.Identity.Host); err == nil {
					break
				}
				select {
				case <-time.After(20 * time.Millisecond):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return verify(ctx)
		})
	}
	if err != nil || !used {
		t.Fatalf("historical containment observation: used=%v err=%v", used, err)
	}
	for {
		select {
		case _, ok := <-pending:
			if !ok {
				return
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

func historicalStoreFiles(state string) (map[string][32]byte, error) {
	result := map[string][32]byte{}
	err := filepath.WalkDir(state, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		// Credential/config/registry files are not persistence; hash SQLite and WAL.
		if !strings.Contains(entry.Name(), ".db") && !strings.Contains(entry.Name(), ".sqlite") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[path] = sha256.Sum256(b)
		return nil
	})
	if err == nil && len(result) == 0 {
		err = fmt.Errorf("no store files selected")
	}
	return result, err
}

func historicalCompletedTask(t *testing.T, ctx context.Context, state string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(state, "conversations.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var status, result, taskError string
		err = db.QueryRowContext(ctx, "SELECT status,result_json,error FROM delegated_tasks LIMIT 1").Scan(&status, &result, &taskError)
		if err == nil && status == "completed" && strings.Contains(result, "native-controller-ok") {
			t.Log("host-owned independent task callback persisted completed result")
			return
		}
		if time.Now().After(deadline) {
			logs, _ := filepath.Glob(filepath.Join(state, "hosts", "*.log"))
			for _, path := range logs {
				b, _ := os.ReadFile(path)
				t.Logf("host log: %s", b)
			}
			t.Fatalf("real task callback missing: status=%q result=%q taskError=%q err=%v", status, result, taskError, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
