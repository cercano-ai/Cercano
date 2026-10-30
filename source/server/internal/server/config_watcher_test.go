package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/pkg/config"
)

// Keep routing services wired: a locus reload must publish a fresh graph even
// when no cloud profile is configured. Isolate diagnostic files from the host.
func newConfigWatcherTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	return newServerWithLogs(nil, &fakeRouter{}, nil, nil, nil,
		filepath.Join(dir, "routing.jsonl"), filepath.Join(dir, "failures.jsonl"))
}

// reloadConfigFromDisk picks up an out-of-band edit, applies hot-reloadable
// fields through UpdateConfig, and broadcasts a ConfigChanged event per
// applied field. Locus changes also rebuild the provider graph, so the fixture
// uses production service wiring with a fake router and no cloud credentials.
func TestReloadConfigFromDisk_AppliesAndBroadcasts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Seed currentConfig from Defaults() so config.Load()'s default-fill
	// doesn't show up as spurious diffs against zero-valued fields.
	base, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	initial := config.Defaults().WithSnapshot(base.Snapshot())
	initial.LocusMode = "cloud_primary"
	if next, err := config.SaveSnapshot(initial, path, initial.Snapshot()); err != nil {
		t.Fatalf("seed config: %v", err)
	} else {
		initial = initial.WithSnapshot(next)
	}
	// Normalize through Load so currentConfig carries the same finalized
	// tier slots a real boot would — otherwise the reload diff sees a
	// spurious everyday.open change (Defaults() leaves tiers empty) and
	// routes into the openProvider path this test deliberately avoids.
	initial, err = config.Load(path)
	if err != nil {
		t.Fatalf("normalize seed: %v", err)
	}

	srv := newConfigWatcherTestServer(t)
	srv.SetConfigPersistence(path, initial)

	ch, unsub := srv.events.subscribe()
	defer unsub()

	// Out-of-band edit: change locus_mode and rewrite the file.
	edited := initial
	edited.LocusMode = "open_primary"
	if next, err := config.SaveSnapshot(edited, path, edited.Snapshot()); err != nil {
		t.Fatalf("edit config: %v", err)
	} else {
		edited = edited.WithSnapshot(next)
	}

	srv.reloadConfigFromDisk(context.Background())

	if got := string(srv.DispatchCandidates().Mode); got != "open_primary" {
		t.Errorf("dispatch mode = %q, want open_primary", got)
	}

	if got := srv.cfgSvc.Get().LocusMode; got != "open_primary" {
		t.Errorf("cfgSvc.Get().LocusMode = %q, want open_primary", got)
	}

	select {
	case ev := <-ch:
		cc := ev.GetConfigChanged()
		if cc == nil {
			t.Fatalf("expected ConfigChanged event, got %T", ev.Event)
		}
		if cc.Field != "locus_mode" || cc.Value != "open_primary" {
			t.Errorf("event = {field:%q value:%q}, want {locus_mode open_primary}", cc.Field, cc.Value)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected ConfigChanged event, none received")
	}
}

// A no-op reload (file content matches currentConfig) must not broadcast.
// This is what keeps the fsnotify echo loop from spamming clients after
// UpdateConfig persists its own change.
func TestReloadConfigFromDisk_NoChangeNoBroadcast(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	base, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults().WithSnapshot(base.Snapshot())
	cfg.LocusMode = "cloud_primary"
	if next, err := config.SaveSnapshot(cfg, path, cfg.Snapshot()); err != nil {
		t.Fatalf("seed config: %v", err)
	} else {
		cfg = cfg.WithSnapshot(next)
	}
	// Normalize through Load (see AppliesAndBroadcasts) so the reload diff
	// doesn't see a spurious everyday.open change against empty tier slots.
	cfg, err = config.Load(path)
	if err != nil {
		t.Fatalf("normalize seed: %v", err)
	}

	srv := newConfigWatcherTestServer(t)
	srv.SetConfigPersistence(path, cfg)

	ch, unsub := srv.events.subscribe()
	defer unsub()

	srv.reloadConfigFromDisk(context.Background())

	select {
	case ev := <-ch:
		t.Errorf("expected no broadcast, got event %T", ev.Event)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

// StartConfigWatcher is non-fatal on a missing config path: returning nil
// matches the permission watcher's behavior so callers don't have to special-
// case the unconfigured case.
func TestStartConfigWatcher_EmptyPathIsNoop(t *testing.T) {
	srv := &Server{cfgSvc: cfgsvc.New("", config.Config{}, nil)}
	if err := srv.StartConfigWatcher(context.Background(), ""); err != nil {
		t.Errorf("StartConfigWatcher(\"\") = %v, want nil", err)
	}
}

// End-to-end: spinning watcher reacts to a real file write within a short
// budget. Guards the wiring between fsnotify, the directory-watch trick, and
// reloadConfigFromDisk.
func TestStartConfigWatcher_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	base, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults().WithSnapshot(base.Snapshot())
	cfg.LocusMode = "cloud_primary"
	if next, err := config.SaveSnapshot(cfg, path, cfg.Snapshot()); err != nil {
		t.Fatalf("seed config: %v", err)
	} else {
		cfg = cfg.WithSnapshot(next)
	}
	// Normalize through Load (see AppliesAndBroadcasts) so the reload diff
	// doesn't see a spurious everyday.open change against empty tier slots.
	cfg, err = config.Load(path)
	if err != nil {
		t.Fatalf("normalize seed: %v", err)
	}

	srv := newConfigWatcherTestServer(t)
	srv.SetConfigPersistence(path, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel + drain BEFORE TempDir cleanup runs: the watcher goroutine may
	// be mid-writeback (UpdateConfig → config.Save) when the test returns,
	// and racing that against rm -r on the watched dir causes "directory not
	// empty". A short sleep is the simplest expression of "let the watcher
	// finish what it's doing"; making the watcher synchronously joinable
	// would be over-engineering for production where it's fire-and-forget.
	defer func() {
		cancel()
		time.Sleep(100 * time.Millisecond)
	}()
	if err := srv.StartConfigWatcher(ctx, path); err != nil {
		t.Fatalf("StartConfigWatcher: %v", err)
	}

	ch, unsub := srv.events.subscribe()
	defer unsub()

	// Give the watcher goroutine a moment to register; then write a change.
	time.Sleep(50 * time.Millisecond)
	edited := cfg
	edited.LocusMode = "open_only"
	if next, err := config.SaveSnapshot(edited, path, edited.Snapshot()); err != nil {
		t.Fatalf("edit config: %v", err)
	} else {
		edited = edited.WithSnapshot(next)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			cc := ev.GetConfigChanged()
			if cc != nil && cc.Field == "locus_mode" && cc.Value == "open_only" {
				return // success
			}
		case <-deadline:
			t.Fatalf("watcher did not deliver locus_mode change within 2s (LocusMode=%q)", srv.cfgSvc.Get().LocusMode)
		}
	}
}
