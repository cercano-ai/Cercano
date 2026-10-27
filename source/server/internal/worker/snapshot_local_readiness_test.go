package worker

import (
	"context"
	"testing"

	"cercano/source/server/internal/dispatch"
	"cercano/source/server/pkg/config"
	workerproto "cercano/source/server/pkg/proto"
	"google.golang.org/protobuf/proto"
)

// Selection must agree with the host after the real per-turn wire round trip.
// Neither provider selection nor this test performs inference or opens credentials.
func TestSnapshotLocalProviderReadiness(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.OpenRuntime = "llama_server"
			cfg.LocusMode = "open_only"
			cfg.LlamaServer.Enabled = enabled
			cfg.TaskAssignments = map[config.Task]config.TaskAssignment{config.TaskChat: {Destination: config.DestinationPrimary, Quality: config.CostPremium}}
			tiers := map[string]string{string(config.TierMostCapable): "offline-fixture"}
			if dispatch.OpenModelReadyFor(cfg, "offline-fixture") != enabled {
				t.Fatal("host readiness setup")
			}
			wire, err := proto.Marshal(SnapshotConfig(cfg, "", tiers))
			if err != nil {
				t.Fatal(err)
			}
			var snapshot workerproto.ConfigSnapshot
			if err := proto.Unmarshal(wire, &snapshot); err != nil {
				t.Fatal(err)
			}
			restored := ConfigFromSnapshot(&snapshot)
			if restored.LlamaServer.Enabled != enabled {
				t.Fatal("runtime admission flag lost on wire")
			}
			resolver, err := buildWorkerProviders(context.Background(), restored, nil, &streamOpenProvider{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			provider, cloud, _, err := resolver.Main()
			if enabled && (err != nil || provider == nil || cloud) {
				t.Fatalf("enabled host provider lost at worker: provider=%v cloud=%v err=%v", provider, cloud, err)
			}
			if !enabled {
				const want = `locus mode "open_only": no local provider available for main work`
				if err == nil || err.Error() != want {
					t.Fatalf("disabled local runtime: got %v, want %s", err, want)
				}
			}
		})
	}
}
