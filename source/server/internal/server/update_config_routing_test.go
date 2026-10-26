package server

import (
	"cercano/source/server/internal/credentialtest"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/locus"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

func routingUpdateServer(t *testing.T, mode string) *Server {
	t.Helper()
	s, _ := newTestServer()
	s.cfgSvc.SetPath(filepath.Join(t.TempDir(), "config.yaml"))
	c := s.cfgSvc.Get()
	c.LocusMode = mode
	c.LocalRedirect = config.DestinationPrimary
	c.SecondaryRedirect = config.DestinationPrimary
	c.ActiveCloudProfile = "messages-one"
	c.ModelProfiles = config.Defaults().ModelProfiles
	if err := s.cfgSvc.Set(c); err != nil {
		t.Fatal(err)
	}
	if err := s.cfgSvc.Secrets().Set("messages-one", "fixture-key"); err != nil {
		t.Fatal(err)
	}
	if err := s.rebuildCloud(); err != nil {
		t.Fatal(err)
	}
	s.SetOpenLLMProvider(&watchdogTaskProvider{})
	return s
}

func TestUpdateConfigLocusRefreshesDispatchSnapshot(t *testing.T) {
	credentialtest.Enable(t)
	s := routingUpdateServer(t, "open_only")
	for _, mode := range []string{"cloud_only", "open_primary", "cloud_primary", "open_only", "cloud_only"} {
		reply, err := s.UpdateConfig(t.Context(), &proto.UpdateConfigRequest{LocusMode: mode})
		if err != nil || !reply.Success {
			t.Fatalf("update %s: %v %+v", mode, err, reply)
		}
		snapshot := s.DispatchCandidates()
		if string(snapshot.Mode) != mode {
			t.Errorf("saved mode %s, stale dispatch snapshot %s", mode, snapshot.Mode)
		}
		current, _ := locus.ParseMode(s.LocusMode())
		for _, dest := range []config.Destination{config.DestinationPrimary, config.DestinationSecondary, config.DestinationLocal} {
			selected, err := inference.SelectDestination(current, dest, snapshot)
			if err != nil {
				t.Errorf("%s/%s: %v", mode, dest, err)
				continue
			}
			wantCloud := mode == "cloud_only" || mode == "cloud_primary"
			if selected.IsCloud != wantCloud {
				t.Errorf("%s/%s selected cloud=%v, want %v", mode, dest, selected.IsCloud, wantCloud)
			}
		}
	}
}

func TestUpdateConfigCloudOnlyDispatchNeverCallsLocal(t *testing.T) {
	credentialtest.Enable(t)
	s := routingUpdateServer(t, "open_only")
	reply, err := s.UpdateConfig(t.Context(), &proto.UpdateConfigRequest{LocusMode: "cloud_only"})
	if err != nil || !reply.Success {
		t.Fatalf("update: %v %+v", err, reply)
	}
	// Replace transports only, retaining the routing graph captured by UpdateConfig.
	// Neither fixture performs network requests or starts a local runtime.
	cloud, local := &watchdogTaskProvider{}, &watchdogTaskProvider{}
	s.SetCloudLLMProvider(cloud)
	s.SetOpenLLMProvider(local)
	engine := dispatch.NewEngine(s.DispatchCandidates, func() locus.Mode { m, _ := locus.ParseMode(s.LocusMode()); return m }, nil)
	result, err := engine.Dispatch(t.Context(), dispatch.Spec{Mode: dispatch.OneShot, Role: dispatch.RoleCoproc, RoutingTask: config.TaskReconnaissance, Prompt: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsCloud || len(cloud.calls) != 1 || len(local.calls) != 0 {
		t.Fatalf("cloud-only dispatch: isCloud=%v cloud calls=%d local calls=%d", result.IsCloud, len(cloud.calls), len(local.calls))
	}
}

func TestUpdateConfigSameLocusRepairsStaleSnapshot(t *testing.T) {
	credentialtest.Enable(t)
	s := routingUpdateServer(t, "open_only")
	// Simulate an already-saved configuration with the old provider graph.
	c := s.cfgSvc.Get()
	c.LocusMode = "cloud_only"
	if err := s.cfgSvc.Set(c); err != nil {
		t.Fatal(err)
	}
	reply, err := s.UpdateConfig(t.Context(), &proto.UpdateConfigRequest{LocusMode: "cloud_only"})
	if err != nil || !reply.Success {
		t.Fatalf("update: %v %+v", err, reply)
	}
	if s.DispatchCandidates().Mode != locus.CloudOnly {
		t.Fatal("reapplying cloud-only did not repair the stale graph")
	}
}

func TestUpdateConfigCloudOnlyMissingAccountDoesNotKeepLocalPolicy(t *testing.T) {
	credentialtest.Enable(t)
	s := routingUpdateServer(t, "open_only")
	c := s.cfgSvc.Get()
	c.ActiveCloudProfile = ""
	c.CloudProfiles = nil
	if err := s.cfgSvc.Set(c); err != nil {
		t.Fatal(err)
	}
	reply, err := s.UpdateConfig(t.Context(), &proto.UpdateConfigRequest{LocusMode: "cloud_only"})
	if err != nil || !reply.Success {
		t.Fatalf("mode must still save: %v %+v", err, reply)
	}
	if !strings.Contains(reply.Message, "cloud unavailable") {
		t.Fatal("unavailable cloud warning missing from successful save response")
	}
	snapshot := s.DispatchCandidates()
	if snapshot.Mode != locus.CloudOnly {
		t.Errorf("unavailable cloud left old mode %s", snapshot.Mode)
	}
	if selected, err := inference.SelectDestination(locus.CloudOnly, config.DestinationPrimary, snapshot); err == nil {
		t.Fatalf("missing cloud should fail closed, got %+v", selected)
	}
}

func TestUpdateConfigUnrelatedSettingKeepsProviderGraph(t *testing.T) {
	credentialtest.Enable(t)
	s := routingUpdateServer(t, "cloud_only")
	before := s.CloudLLMProvider()
	reply, err := s.UpdateConfig(t.Context(), &proto.UpdateConfigRequest{WatchdogEnabled: "true"})
	if err != nil || !reply.Success {
		t.Fatalf("update: %v %+v", err, reply)
	}
	if s.CloudLLMProvider() != before {
		t.Fatal("unrelated setting rebuilt cloud providers")
	}
}

func TestUpdateConfigInvalidLocusKeepsProviderGraph(t *testing.T) {
	credentialtest.Enable(t)
	s := routingUpdateServer(t, "cloud_only")
	before := s.CloudLLMProvider()
	reply, err := s.UpdateConfig(t.Context(), &proto.UpdateConfigRequest{LocusMode: "invalid-mode"})
	if err != nil || reply.Success {
		t.Fatalf("invalid mode response: %v %+v", err, reply)
	}
	if s.CloudLLMProvider() != before || s.DispatchCandidates().Mode != locus.CloudOnly || s.LocusMode() != "cloud_only" {
		t.Fatal("invalid mode mutated routing")
	}
}
