package server

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

// Two ordinary Servers, independent config services, one fixture authority.
// No startup, native credentials, runtime process or provider transport.
func crossHostServers(t *testing.T) (*Server, *Server) {
	t.Helper()
	a, _ := counterexampleServer(t)
	b := newConfigWatcherTestServer(t)
	c, err := config.Load(a.cfgSvc.Path())
	if err != nil {
		t.Fatal(err)
	}
	b.SetConfigPersistence(a.cfgSvc.Path(), c)
	b.cfgSvc = counterexampleConfig{b.cfgSvc, secrets.NewMemory()}
	return a, b
}

func TestCrossHostConfigFailedApplyOutcome(t *testing.T) {
	a, _ := crossHostServers(t)
	a.providerSvc = removalRebuildFailure{a.providerSvc}
	events, unsub := a.events.subscribe()
	defer unsub()
	r, err := a.UpdateConfig(context.Background(), &proto.UpdateConfigRequest{LocusMode: "open_only"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Success || !strings.Contains(r.Message, "configuration saved but not applied") {
		t.Fatalf("failed rebuild acknowledged usable policy: %+v", r)
	}
	select {
	case ev := <-events:
		t.Fatalf("failed application acknowledged: %v", ev)
	default:
	}
	disk, err := config.Load(a.cfgSvc.Path())
	if err != nil {
		t.Fatal(err)
	}
	if disk.LocusMode != "open_only" || disk.Snapshot().Revision != a.cfgSvc.Get().Snapshot().Revision {
		t.Fatal("durable save outcome lost")
	}
}

func TestCrossHostConfigConvergence(t *testing.T) {
	for _, rpc := range []bool{false, true} {
		name := "SaveAndSet"
		if rpc {
			name = "UpdateConfig"
		}
		t.Run(name, func(t *testing.T) {
			a, b := crossHostServers(t)
			if a.cfgSvc == b.cfgSvc {
				t.Fatal("shared service")
			}
			if rpc {
				r, err := a.UpdateConfig(context.Background(), &proto.UpdateConfigRequest{ToolLoopMaxIterations: "37"})
				if err != nil || !r.Success {
					t.Fatalf("save: %+v %v", r, err)
				}
			} else {
				draft := a.cfgSvc.Get()
				draft.ToolLoop.MaxIterations = 37
				if err := a.cfgSvc.SaveAndSet(draft); err != nil {
					t.Fatal(err)
				}
			}
			path := a.cfgSvc.Path()
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			events, unsub := b.events.subscribe()
			defer unsub()
			b.reloadConfigFromDisk(context.Background())
			if b.cfgSvc.Get().Snapshot().Revision != a.cfgSvc.Get().Snapshot().Revision || b.cfgSvc.Get().ToolLoop.MaxIterations != 37 {
				t.Fatal("B did not adopt exact A snapshot")
			}
			after, _ := os.ReadFile(path)
			afterInfo, _ := os.Stat(path)
			if !bytes.Equal(before, after) || !os.SameFile(info, afterInfo) || !info.ModTime().Equal(afterInfo.ModTime()) {
				t.Fatal("reload resaved authority")
			}
			select {
			case <-events:
			default:
				t.Fatal("missing application event")
			}
			for len(events) > 0 {
				<-events
			}
			b.reloadConfigFromDisk(context.Background())
			select {
			case ev := <-events:
				t.Fatalf("echo event: %v", ev)
			default:
			}
			draft := b.cfgSvc.Get()
			draft.ToolLoop.MaxIterations = 38
			if err := b.cfgSvc.SaveAndSet(draft); err != nil {
				t.Fatalf("adopted baseline unusable: %v", err)
			}
			disk, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if disk.ToolLoop.MaxIterations != 38 || disk.Snapshot().Revision != b.cfgSvc.Get().Snapshot().Revision {
				t.Fatal("B save not published")
			}
			a.reloadConfigFromDisk(context.Background())
			if a.cfgSvc.Get().Snapshot().Revision != disk.Snapshot().Revision {
				t.Fatal("A did not converge back")
			}
		})
	}
}

func TestCrossHostConfigConcurrentDraftConflict(t *testing.T) {
	a, b := crossHostServers(t)
	base := a.cfgSvc.Get().Snapshot().Revision
	da, db := a.cfgSvc.Get(), b.cfgSvc.Get()
	da.ToolLoop.MaxIterations = 41
	db.ToolLoop.MaxIterations = 42
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, item := range []struct {
		s cfgsvc.Service
		c config.Config
	}{{a.cfgSvc, da}, {b.cfgSvc, db}} {
		go func(s cfgsvc.Service, c config.Config) { <-start; results <- s.SaveAndSet(c) }(item.s, item.c)
	}
	close(start)
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("want one success, one conflict: %v / %v", first, second)
	}
	conflict := first
	if conflict == nil {
		conflict = second
	}
	if !errors.Is(conflict, config.ErrConfigConflict) {
		t.Fatalf("not conflict: %v", conflict)
	}
	loser := a
	if b.cfgSvc.Get().Snapshot().Revision == base {
		loser = b
	}
	if loser.cfgSvc.Get().Snapshot().Revision != base {
		t.Fatal("stale draft silently refreshed")
	}
	a.reloadConfigFromDisk(context.Background())
	b.reloadConfigFromDisk(context.Background())
	if a.cfgSvc.Get().Snapshot().Revision != b.cfgSvc.Get().Snapshot().Revision {
		t.Fatal("conflict recovery did not converge")
	}
}

// Existing reload error API is a diagnostic, not an RPC/event outcome.
// Capture it synchronously; these tests deliberately do not run in parallel.
func captureConfigDiagnostic(t *testing.T, f func()) string {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "diagnostic")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	old := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = old }()
	f()
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCrossHostConfigInvalidYAMLRetainsLastKnown(t *testing.T) {
	a, b := crossHostServers(t)
	before := b.cfgSvc.Get()
	events, unsub := b.events.subscribe()
	defer unsub()
	invalid := []byte("models: [unterminated\n")
	if err := os.WriteFile(a.cfgSvc.Path(), invalid, 0600); err != nil {
		t.Fatal(err)
	}
	diagnostic := captureConfigDiagnostic(t, func() { b.reloadConfigFromDisk(context.Background()) })
	if !strings.Contains(diagnostic, "failed to reload") {
		t.Fatalf("missing visible error: %q", diagnostic)
	}
	if b.cfgSvc.Get().Snapshot().Revision != before.Snapshot().Revision {
		t.Fatal("load failure adopted/reset baseline")
	}
	select {
	case ev := <-events:
		t.Fatalf("invalid YAML acknowledged: %v", ev)
	default:
	}
	data, _ := os.ReadFile(a.cfgSvc.Path())
	if !bytes.Equal(data, invalid) {
		t.Fatal("invalid file overwritten")
	}
	if err := b.cfgSvc.SaveAndSet(before); err == nil {
		t.Fatal("stale save replaced invalid external edit")
	}
}
