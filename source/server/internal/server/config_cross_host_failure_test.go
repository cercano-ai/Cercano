package server

import (
	"context"
	"strings"
	"testing"

	"cercano/source/server/pkg/proto"
)

func TestCrossHostConfigWatcherFailedApply(t *testing.T) {
	a, b := crossHostServers(t)
	// Establish an old usable graph using the ordinary apply path first.
	r, err := b.UpdateConfig(context.Background(), &proto.UpdateConfigRequest{LocusMode: "open_only"})
	if err != nil || !r.Success {
		t.Fatalf("initial apply: %+v %v", r, err)
	}
	a.reloadConfigFromDisk(context.Background())
	oldMode := b.DispatchCandidates().Mode
	b.providerSvc = removalRebuildFailure{b.providerSvc}
	events, unsub := b.events.subscribe()
	defer unsub()
	draft := a.cfgSvc.Get()
	draft.LocusMode = "cloud_only"
	if err := a.cfgSvc.SaveAndSet(draft); err != nil {
		t.Fatal(err)
	}
	diagnostic := captureConfigDiagnostic(t, func() { b.reloadConfigFromDisk(context.Background()) })
	if !strings.Contains(diagnostic, "configuration saved but not applied: injected rebuild failure") {
		t.Fatalf("missing apply failure: %q", diagnostic)
	}
	select {
	case ev := <-events:
		t.Fatalf("failed application acknowledged: %v", ev)
	default:
	}
	if b.cfgSvc.Get().Snapshot().Revision != a.cfgSvc.Get().Snapshot().Revision {
		t.Fatal("saved baseline was not adopted")
	}
	// Explicit qualification boundary: baseline adoption is not policy freshness.
	// Existing admission still exposes the old graph, and an identical revision
	// reload is skipped. This test records the gap, not cross-host certification.
	if b.DispatchCandidates().Mode != oldMode {
		t.Fatal("fixture did not retain old graph")
	}
	diagnostic = captureConfigDiagnostic(t, func() { b.reloadConfigFromDisk(context.Background()) })
	if diagnostic != "" {
		t.Fatalf("same revision unexpectedly retried: %q", diagnostic)
	}
}
