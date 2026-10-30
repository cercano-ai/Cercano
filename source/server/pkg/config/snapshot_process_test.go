package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/filelock"
	"cercano/source/server/internal/secrets"
)

const snapshotProcessHelperEnv = "CERCANO_SNAPSHOT_PROCESS_HELPER"

// The wire protocol is deliberately separate from go test's final PASS output.
// Every read is bounded; CommandContext also bounds children blocked inside the
// context-free SaveSnapshot API. No application, provider, or credential store
// is started by these helpers.
type snapshotProcessMessage struct {
	Phase    string
	Data     []byte
	Revision ConfigRevision
	Absent   bool
}

type snapshotProcessRead struct {
	message snapshotProcessMessage
	err     error
}

func snapshotProcessReceive(t *testing.T, ctx context.Context, reads <-chan snapshotProcessRead, phase string) snapshotProcessMessage {
	t.Helper()
	select {
	case result := <-reads:
		if result.err != nil {
			t.Fatalf("waiting for %s: %v", phase, result.err)
		}
		if result.message.Phase != phase {
			t.Fatalf("got phase %q, want %q", result.message.Phase, phase)
		}
		return result.message
	case <-ctx.Done():
		t.Fatalf("waiting for %s: %v", phase, ctx.Err())
		return snapshotProcessMessage{}
	}
}

func snapshotProcessReads(ctx context.Context, reader io.Reader) <-chan snapshotProcessRead {
	out := make(chan snapshotProcessRead)
	go func() {
		decoder := json.NewDecoder(reader)
		for {
			var result snapshotProcessRead
			result.err = decoder.Decode(&result.message)
			select {
			case out <- result:
			case <-ctx.Done():
				return
			}
			if result.err != nil {
				return
			}
		}
	}()
	return out
}

func snapshotProcessSend(t *testing.T, encoder *json.Encoder, message snapshotProcessMessage) {
	t.Helper()
	if err := encoder.Encode(message); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotProcessHelper(t *testing.T) {
	if os.Getenv(snapshotProcessHelperEnv) != "1" {
		t.Skip("test-binary subprocess only")
	}
	if os.Getenv(secrets.DisableEnv) != "1" || !errors.Is(secrets.CheckAllowed(), secrets.ErrDisabled) {
		t.Fatal("helper requires explicitly disabled credentials")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	reads := snapshotProcessReads(ctx, os.Stdin)
	output := json.NewEncoder(os.Stdout)
	path := os.Getenv("CERCANO_SNAPSHOT_PROCESS_PATH")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	base := cfg.Snapshot()
	stale := cfg // This value must never be advanced by the winning save.
	cfg.Port = os.Getenv("CERCANO_SNAPSHOT_PROCESS_PORT")
	snapshotProcessSend(t, output, snapshotProcessMessage{Phase: "loaded", Data: base.data, Revision: base.Revision, Absent: base.absent})

	// The parent now owns the OS lock. A process-local mutex cannot cause
	// this deadline: Save must honor another process's lock.
	snapshotProcessReceive(t, ctx, reads, "probe")
	coordinator, err := cfg.PrepareConfigCoordination(path)
	if err != nil {
		t.Fatal(err)
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	_, err = coordinator.Save(probeCtx, base, cfg)
	probeCancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Save bypassed cross-process lock: %v", err)
	}
	snapshotProcessSend(t, output, snapshotProcessMessage{Phase: "blocked"})

	// Both independent processes have loaded the same baseline before the
	// parent releases this publication barrier. Neither refreshes its base.
	snapshotProcessReceive(t, ctx, reads, "save")
	next, err := SaveSnapshot(cfg, path, base)
	switch {
	case err == nil:
		snapshotProcessSend(t, output, snapshotProcessMessage{Phase: "won", Data: next.data, Revision: next.Revision})
	case errors.Is(err, ErrConfigConflict):
		snapshotProcessSend(t, output, snapshotProcessMessage{Phase: "conflict"})
		snapshotProcessReceive(t, ctx, reads, "finish")
		return
	default:
		t.Fatalf("SaveSnapshot: %v", err)
	}

	// Keep the first winner's bytes stable until the parent has inspected
	// both outcomes. Then prove only the returned baseline permits progress.
	snapshotProcessReceive(t, ctx, reads, "advance")
	cfg = cfg.WithSnapshot(next)
	cfg.Port += "-next"
	advanced, err := SaveSnapshot(cfg, path, cfg.Snapshot())
	if err != nil {
		t.Fatalf("save with returned baseline: %v", err)
	}
	if _, err := SaveSnapshot(stale, path, stale.Snapshot()); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("copied stale draft accepted: %v", err)
	}
	if _, err := SaveSnapshot(cfg, path, next); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("previous winning baseline accepted after advance: %v", err)
	}
	snapshotProcessSend(t, output, snapshotProcessMessage{Phase: "advanced", Data: advanced.data, Revision: advanced.Revision})
	snapshotProcessReceive(t, ctx, reads, "finish")
}

type snapshotProcessChild struct {
	wait  func() error
	input *json.Encoder
	reads <-chan snapshotProcessRead
}

func startSnapshotProcess(t *testing.T, ctx context.Context, path, port string) *snapshotProcessChild {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, dir := range []string{"home", "xdg", "cache", "data", "tmp"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	childCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(childCtx, executable, "-test.run=^TestSnapshotProcessHelper$", "-test.timeout=30s")
	// Do not inherit provider keys, native config locations, or app settings.
	cmd.Env = []string{
		snapshotProcessHelperEnv + "=1", secrets.DisableEnv + "=1",
		"CERCANO_SNAPSHOT_PROCESS_PATH=" + path, "CERCANO_SNAPSHOT_PROCESS_PORT=" + port,
		"HOME=" + filepath.Join(root, "home"), "XDG_CONFIG_HOME=" + filepath.Join(root, "xdg"),
		"XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "XDG_DATA_HOME=" + filepath.Join(root, "data"),
		"TMPDIR=" + filepath.Join(root, "tmp"), "GOPROXY=off", "GOSUMDB=off",
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		cancel() // Only this owned exec.Cmd is killed, never process groups.
		_ = input.Close()
		if !waited {
			_ = cmd.Wait()
		}
		if stderr.Len() != 0 {
			t.Logf("child %s stderr: %s", port, stderr.String())
		}
	})
	// Wait is done by cleanup on early failures; successful paths explicitly
	// wait and check the exit status before marking the child reaped.
	child := &snapshotProcessChild{input: json.NewEncoder(input), reads: snapshotProcessReads(childCtx, output)}
	child.wait = func() error {
		err := cmd.Wait()
		waited = true
		return err
	}
	return child
}

func TestSnapshotProcessCoordination(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%v", existing), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "config.yaml")
			var initial []byte
			if existing {
				initial = []byte("port: '1111'\n")
				if err := os.WriteFile(path, initial, 0600); err != nil {
					t.Fatal(err)
				}
			}
			children := []*snapshotProcessChild{
				startSnapshotProcess(t, ctx, path, "2222"),
				startSnapshotProcess(t, ctx, path, "3333"),
			}
			var baseline ConfigRevision
			for i, child := range children {
				loaded := snapshotProcessReceive(t, ctx, child.reads, "loaded")
				if loaded.Absent != !existing || !bytes.Equal(loaded.Data, initial) {
					t.Fatalf("child %d loaded wrong baseline: %+v", i, loaded)
				}
				if i == 0 {
					baseline = loaded.Revision
				} else if loaded.Revision != baseline {
					t.Fatal("children did not load identical baselines")
				}
			}
			release, err := filelock.Lock(ctx, path+".lock")
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			for _, child := range children {
				snapshotProcessSend(t, child.input, snapshotProcessMessage{Phase: "probe"})
			}
			for _, child := range children {
				snapshotProcessReceive(t, ctx, child.reads, "blocked")
			}
			data, readErr := os.ReadFile(path)
			if existing && (readErr != nil || !bytes.Equal(data, initial)) || !existing && !os.IsNotExist(readErr) {
				t.Fatalf("blocked saves changed authority: %q, %v", data, readErr)
			}
			release()
			// Send both before receiving either result: no winner-order sleeps.
			for _, child := range children {
				snapshotProcessSend(t, child.input, snapshotProcessMessage{Phase: "save"})
			}
			winner, wins, conflicts := -1, 0, 0
			var winning snapshotProcessMessage
			for i, child := range children {
				select {
				case result := <-child.reads:
					if result.err != nil {
						t.Fatal(result.err)
					}
					switch result.message.Phase {
					case "won":
						winner, winning = i, result.message
						wins++
					case "conflict":
						conflicts++
					default:
						t.Fatalf("unexpected save outcome: %+v", result.message)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			if wins != 1 || conflicts != 1 {
				t.Fatalf("winning publications=%d conflicts=%d, want exactly 1 each", wins, conflicts)
			}
			assertBytes := func(want []byte) {
				t.Helper()
				got, err := os.ReadFile(path)
				if err != nil || len(want) == 0 || !bytes.Equal(got, want) {
					t.Fatalf("winner bytes not intact: read=%v equal=%v", err, bytes.Equal(got, want))
				}
			}
			assertBytes(winning.Data)
			snapshotProcessSend(t, children[winner].input, snapshotProcessMessage{Phase: "advance"})
			advanced := snapshotProcessReceive(t, ctx, children[winner].reads, "advanced")
			if advanced.Revision == winning.Revision {
				t.Fatal("sequential save did not advance revision")
			}
			assertBytes(advanced.Data) // Includes both rejected stale-save attempts.
			for _, child := range children {
				snapshotProcessSend(t, child.input, snapshotProcessMessage{Phase: "finish"})
			}
			for _, child := range children {
				if err := child.wait(); err != nil {
					t.Fatalf("helper exit: %v", err)
				}
			}
			t.Logf("contested phase: exactly %d winner, %d conflict; returned baseline advanced; stale copies rejected", wins, conflicts)
		})
	}
}
