package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/locus"
	"cercano/source/server/internal/loopcompact"
	"cercano/source/server/internal/managedsettings"
	"cercano/source/server/internal/managedsettings/settingstest"
	"cercano/source/server/pkg/config"
	v1 "github.com/bryancostanich/Cercano/source/enterpriseapi/v1"
)

func TestRecapRoutePrecedence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	for _, tc := range []struct {
		name            string
		override, graph bool
		source          string
	}{
		{"default", false, false, "task_default"}, {"override", true, false, "task_override"}, {"graph_beats_stale_config", true, true, "task_graph_override"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.LocusMode = "cloud_only"
			dest := config.DestinationSecondary
			if tc.override {
				dest = config.DestinationPrimary
				cfg.TaskAssignments = map[config.Task]config.TaskAssignment{config.TaskCompaction: {Destination: dest, Quality: config.CostEconomy}}
			}
			cloud := &schedulingProvider{}
			tiers := inference.Tiers{Mode: locus.CloudOnly, Cloud: cloud, Destinations: map[config.Destination]inference.Candidate{
				config.DestinationPrimary: {Provider: cloud, Profile: "primary-fixture", IsCloud: true}, config.DestinationSecondary: {Provider: cloud, Profile: "secondary-fixture", IsCloud: true},
			}, ModelFor: func(s inference.Selection, _ config.Tier) string { return string(s.Destination) + "-mock-model" }}
			if tc.graph {
				dest = config.DestinationSecondary
				graph := config.Defaults()
				graph.TaskAssignments = map[config.Task]config.TaskAssignment{config.TaskCompaction: {Destination: dest, Quality: config.CostEconomy}}
				tiers.DeveloperConfig = &graph
				tiers.TaskFor = graph.TaskAssignment
			}
			log, err := os.CreateTemp(t.TempDir(), "route")
			if err != nil {
				t.Fatal(err)
			}
			previous := os.Stderr
			os.Stderr = log
			_, runner, model, _, err := resolveRecapRoute(context.Background(), loopcompact.WiringDeps{Cfg: cfg, Candidates: func() inference.Tiers { return tiers }})
			os.Stderr = previous
			log.Close()
			data, readErr := os.ReadFile(log.Name())
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err != nil {
				t.Fatal(err)
			}
			if runner == nil || model != string(dest)+"-mock-model" {
				t.Fatalf("model=%s", model)
			}
			diagnostic := string(data)
			if !strings.Contains(diagnostic, `source="`+tc.source+`"`) || !strings.Contains(diagnostic, "profile=") || !strings.Contains(diagnostic, "model=") {
				t.Fatalf("provenance: %s", diagnostic)
			}
		})
	}
}

// Assert the actual selected provider changes on the next request, not only its diagnostic label.
func TestRecapRequestUsesCurrentRoute(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	st, err := conversation.Open(t.TempDir() + "/recap.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.EnsureConversation(ctx, "c", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.Append(ctx, conversation.Turn{ID: "t", ConversationID: "c", Role: "user", Content: "fixture", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.LocusMode = "cloud_only"
	cfg.TaskAssignments = map[config.Task]config.TaskAssignment{
		config.Task("recap"): {Destination: config.DestinationLocal, Quality: config.CostEconomy},
	}
	dest := config.DestinationSecondary
	var called []config.Destination
	makeProvider := func(d config.Destination) *schedulingProvider {
		return &schedulingProvider{call: func(context.Context) error { called = append(called, d); return nil }}
	}
	secondary, primary := makeProvider(config.DestinationSecondary), makeProvider(config.DestinationPrimary)
	p := composeProductionScheduling(st, productionSchedulingDependencies{loop: loopcompact.WiringDeps{Cfg: cfg, Candidates: func() inference.Tiers {
		pinned := dest
		return inference.Tiers{Mode: locus.CloudOnly, Cloud: primary, Destinations: map[config.Destination]inference.Candidate{config.DestinationSecondary: {Provider: secondary, Profile: "secondary", IsCloud: true}}, Open: &schedulingProvider{call: func(context.Context) error { t.Error("unexpected local inference"); return nil }}, TaskFor: func(task config.Task) config.TaskAssignment {
			if task != config.TaskCompaction {
				t.Errorf("selected task=%s, want compaction", task)
				return cfg.TaskAssignment(task)
			}
			return config.TaskAssignment{Destination: pinned, Quality: config.CostEconomy}
		}, ModelFor: func(inference.Selection, config.Tier) string { return "cloud-fixture" }}
	}}}, productionSchedulingOptions{})
	defer p.recap.Close(ctx)
	owner := broker.Owner{Conversation: "c", Host: "mock", Epoch: "mock"}
	admission := broker.NewAdmission("mock")
	if err := admission.RegisterOwner(owner); err != nil {
		t.Fatal(err)
	}
	run := func() {
		t.Helper()
		work, err := p.recap.ScheduleOwned(ctx, owner, func(c context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
			ticket, err := admission.AdmitWork(o, k)
			return c, ticket, err
		})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-work.Done:
		case <-time.After(5 * time.Second):
			t.Fatal("recap hung")
		}
		if work.Err != nil || work.Warning != nil {
			t.Fatalf("err=%v warning=%v", work.Err, work.Warning)
		}
	}
	run()
	firstCalls := len(called)
	dest = config.DestinationPrimary
	run()
	if firstCalls == 0 || len(called) <= firstCalls {
		t.Fatalf("actual provider calls=%v", called)
	}
	for i, d := range called {
		want := config.DestinationSecondary
		if i >= firstCalls {
			want = config.DestinationPrimary
		}
		if d != want {
			t.Fatalf("actual provider calls=%v", called)
		}
	}
}

func TestRecapCloudOnlyRejectsLocal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	cfg := config.Defaults()
	cfg.LocusMode = "cloud_only"
	cfg.TaskAssignments = map[config.Task]config.TaskAssignment{config.TaskCompaction: {Destination: config.DestinationLocal, Quality: config.CostEconomy}}
	local := &schedulingProvider{call: func(context.Context) error { t.Fatal("local inference must not run"); return nil }}
	_, runner, _, _, err := resolveRecapRoute(context.Background(), loopcompact.WiringDeps{Cfg: cfg, Candidates: func() inference.Tiers { return inference.Tiers{Mode: locus.CloudOnly, Open: local} }})
	if err == nil || runner != nil {
		t.Fatalf("cloud-only local selection: runner=%v err=%v", runner, err)
	}
}

// A legacy Recap assignment must not override the configured Compaction route.
func TestRecapUsesCompactionAssignment(t *testing.T) {
	for _, graph := range []bool{false, true} {
		t.Run(map[bool]string{false: "config", true: "graph"}[graph], func(t *testing.T) {
			cfg := config.Defaults()
			cfg.LocusMode = "cloud_only"
			cfg.TaskAssignments = map[config.Task]config.TaskAssignment{
				config.Task("recap"):  {Destination: config.DestinationLocal, Quality: config.CostEconomy},
				config.TaskCompaction: {Destination: config.DestinationPrimary, Quality: config.CostPremium},
			}
			cloud := &schedulingProvider{}
			tiers := inference.Tiers{Mode: locus.CloudOnly, Cloud: cloud, Open: &schedulingProvider{}, ModelFor: func(s inference.Selection, tier config.Tier) string {
				if !s.IsCloud || s.PolicyDestination != config.DestinationPrimary || tier != config.TierMostCapable {
					t.Errorf("selection=%+v tier=%s", s, tier)
				}
				return "compaction-premium"
			}}
			if graph {
				tiers.TaskFor = cfg.TaskAssignment
				tiers.DeveloperConfig = &cfg
			}
			_, runner, model, tier, err := resolveRecapRoute(context.Background(), loopcompact.WiringDeps{Cfg: cfg, Candidates: func() inference.Tiers { return tiers }})
			if err != nil || runner == nil || model != "compaction-premium" || tier != string(config.TierMostCapable) {
				t.Fatalf("runner=%v model=%q tier=%q err=%v", runner, model, tier, err)
			}
		})
	}
}

func TestRecapUsesManagedCompactionRoute(t *testing.T) {
	snapshot := settingstest.Snapshot("fixture", "1", "fixture")
	snapshot.Policy.AllowedRoutes = append(snapshot.Policy.AllowedRoutes, v1.Route{ID: "recap", Provider: "openai", Endpoint: "https://recap.example/v1", Model: "wrong-recap", Placement: "external"})
	snapshot.Policy.TaskDefaults = []v1.TaskDefault{
		{Task: "compaction", Destination: "secondary", Quality: "premium", RouteID: "approved"},
		{Task: "recap", Destination: "primary", Quality: "economy", RouteID: "recap"},
	}
	ctx := managedsettings.WithSnapshot(context.Background(), snapshot)
	cfg := config.Defaults()
	tiers := inference.Tiers{Mode: locus.CloudOnly, ManagedRoute: func(_ context.Context, route v1.Route, dest config.Destination) (inference.Candidate, error) {
		if route.ID != "approved" || dest != config.DestinationSecondary {
			t.Errorf("route=%s destination=%s", route.ID, dest)
		}
		return inference.Candidate{Provider: &schedulingProvider{}, Profile: route.ID, IsCloud: true}, nil
	}, Open: &schedulingProvider{call: func(context.Context) error { t.Fatal("unexpected local inference"); return nil }}}
	_, runner, model, tier, err := resolveRecapRoute(ctx, loopcompact.WiringDeps{Cfg: cfg, Candidates: func() inference.Tiers { return tiers }})
	if err != nil || runner == nil || model != "approved" || tier != string(config.TierMostCapable) {
		t.Fatalf("runner=%v model=%q tier=%q err=%v", runner, model, tier, err)
	}
}
