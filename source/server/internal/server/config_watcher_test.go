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
	initial := config.Defaults()
	initial.LocusMode = "cloud_primary"
	if err := config.Save(initial, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	// Normalize through Load so currentConfig carries the same finalized
	// tier slots a real boot would — otherwise the reload diff sees a
	// spurious everyday.open change (Defaults() leaves tiers empty) and
	// routes into the openProvider path this test deliberately avoids.
	initial, err := config.Load(path)
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
	if err := config.Save(edited, path); err != nil {
		t.Fatalf("edit config: %v", err)
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
	cfg := config.Defaults()
	cfg.LocusMode = "cloud_primary"
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	// Normalize through Load (see AppliesAndBroadcasts) so the reload diff
	// doesn't see a spurious everyday.open change against empty tier slots.
	cfg, err := config.Load(path)
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
	cfg := config.Defaults()
	cfg.LocusMode = "cloud_primary"
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	// Normalize through Load (see AppliesAndBroadcasts) so the reload diff
	// doesn't see a spurious everyday.open change against empty tier slots.
	cfg, err := config.Load(path)
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
	if err := config.Save(edited, path); err != nil {
		t.Fatalf("edit config: %v", err)
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

// A change to machine.yaml (another profile's edit) reaches this agent's
// in-memory config and baseline without a restart, and the agent's next save
// does not write it back as its own edit.
func TestStartConfigWatcher_RefreshesMachineSections(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := config.DefaultPath()
	if err := config.Save(config.Defaults(), path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	srv := newConfigWatcherTestServer(t)
	srv.SetConfigPersistence(path, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		time.Sleep(100 * time.Millisecond)
	}()
	if err := srv.StartConfigWatcher(ctx, path); err != nil {
		t.Fatalf("StartConfigWatcher: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	mc, err := config.LoadMachine()
	if err != nil {
		t.Fatal(err)
	}
	mc.MistralRS.ISQ = "Q4K"
	if err := config.SaveMachine(mc); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for srv.cfgSvc.Get().MistralRS.ISQ != "Q4K" {
		if time.Now().After(deadline) {
			t.Fatal("agent did not pick up machine.yaml change within 2s")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// The machine moves on again; the agent persists an unrelated change.
	mc.MistralRS.ISQ = "Q8_0"
	if err := config.SaveMachine(mc); err != nil {
		t.Fatal(err)
	}
	srv.cfgSvc.Persist()
	got, _ := config.LoadMachine()
	if got.MistralRS.ISQ != "Q8_0" {
		t.Errorf("persist wrote stale machine section back: ISQ=%q", got.MistralRS.ISQ)
	}
}

// The machine.yaml event from this agent's own earlier save can arrive after
// a newer runtime edit was made in memory but before it was persisted. The
// refresh must not revert that edit; fields the edit did not touch still take
// the file's values.
func TestReloadMachineKeepsUnsavedRuntimeEdit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := config.DefaultPath()
	if err := config.Save(config.Defaults(), path); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	srv := newConfigWatcherTestServer(t)
	srv.SetConfigPersistence(path, cfg)

	setISQ := func(v string) {
		if err := srv.cfgSvc.Mutate(func(c *config.Config) { c.MistralRS.ISQ = v }); err != nil {
			t.Fatal(err)
		}
	}
	setISQ("Q4K")
	srv.cfgSvc.Persist() // machine.yaml now has Q4K; its event is still in flight
	setISQ("Q8_0")       // newer edit, not yet persisted

	// Another profile changed an unrelated field meanwhile.
	mc, err := config.LoadMachine()
	if err != nil {
		t.Fatal(err)
	}
	mc.LlamaServer.Threads = 7
	if err := config.SaveMachine(mc); err != nil {
		t.Fatal(err)
	}

	srv.reloadMachineFromDisk() // the delayed event
	live := srv.cfgSvc.Get()
	if live.MistralRS.ISQ != "Q8_0" {
		t.Fatalf("refresh reverted the unsaved edit: ISQ=%q, want Q8_0", live.MistralRS.ISQ)
	}
	if live.LlamaServer.Threads != 7 {
		t.Errorf("refresh dropped the other profile's change: threads=%d, want 7", live.LlamaServer.Threads)
	}
	srv.cfgSvc.Persist()
	got, err := config.LoadMachine()
	if err != nil {
		t.Fatal(err)
	}
	if got.MistralRS.ISQ != "Q8_0" || got.LlamaServer.Threads != 7 {
		t.Errorf("machine.yaml after persist: ISQ=%q threads=%d, want Q8_0 and 7", got.MistralRS.ISQ, got.LlamaServer.Threads)
	}
}
