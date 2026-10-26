package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProductRoutingDefaultsLoading(t *testing.T) {
	for _, tc := range []struct {
		name     string
		yaml     string
		missing  bool
		chat     TaskAssignment
		dispatch TaskAssignment
	}{
		{"missing", "", true, TaskAssignment{DestinationPrimary, CostPremium}, TaskAssignment{DestinationSecondary, CostStandard}},
		{"empty", "{}", false, TaskAssignment{DestinationPrimary, CostPremium}, TaskAssignment{DestinationSecondary, CostStandard}},
		{"legacy", "cloud_model: legacy-model\n", false, TaskAssignment{DestinationPrimary, CostPremium}, TaskAssignment{DestinationSecondary, CostStandard}},
		{"saved", "task_assignments:\n  chat: {destination: local, quality: standard}\n  dispatch: {destination: primary, quality: economy}\n", false, TaskAssignment{DestinationLocal, CostStandard}, TaskAssignment{DestinationPrimary, CostEconomy}},
		{"sparse", "task_assignments:\n  chat: {destination: secondary}\n  dispatch: {quality: premium}\n", false, TaskAssignment{DestinationSecondary, CostPremium}, TaskAssignment{DestinationSecondary, CostPremium}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if !tc.missing {
				if err := os.WriteFile(path, []byte(tc.yaml), 0600); err != nil {
					t.Fatal(err)
				}
			}
			c, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			check := func(c Config) {
				t.Helper()
				if got := c.ResolveTask(TaskChat, ""); got != tc.chat {
					t.Fatalf("chat=%+v, want %+v", got, tc.chat)
				}
				if got := c.ResolveTask(TaskDispatch, ""); got != tc.dispatch {
					t.Fatalf("dispatch=%+v, want %+v", got, tc.dispatch)
				}
			}
			check(c)
			if err := Save(c, path); err != nil {
				t.Fatal(err)
			}
			c, err = Load(path)
			if err != nil {
				t.Fatal(err)
			}
			check(c)
		})
	}
	for _, c := range []Config{{}, Defaults()} {
		if got := c.TaskAssignment(TaskChat); got != (TaskAssignment{DestinationPrimary, CostPremium}) {
			t.Fatalf("chat default=%+v", got)
		}
		if got := c.TaskAssignment(TaskDispatch); got.Destination != DestinationSecondary || got.Quality != CostStandard || got.Quality.CapabilityTier() != TierEveryday {
			t.Fatalf("dispatch default=%+v", got)
		}
	}
}
