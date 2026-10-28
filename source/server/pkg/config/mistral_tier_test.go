package config

import (
	"path/filepath"
	"testing"
)

func TestMistralDefaultRecommendations(t *testing.T) {
	p := CloudProfile{Name: "mistral", Flavor: "chat_completions", BaseURL: "https://api.mistral.ai/v1"}
	check := func(t *testing.T, cfg Config) {
		t.Helper()
		for tier, want := range map[Tier]string{
			TierFastLight:     "mistral-small-latest",
			TierFastLightText: "mistral-small-latest",
			TierEveryday:      "mistral-large-4",
			TierMostCapable:   "zai-glm-5-3",
			TierVision:        "mistral-large-4",
		} {
			if got := cfg.ModelProfiles.ResolveCloudModelForTier(p, tier); got != want {
				t.Errorf("tier %s = %q, want %q", tier, got, want)
			}
		}
	}
	t.Run("fresh defaults", func(t *testing.T) { check(t, Defaults()) })
	t.Run("existing config and explicit override", func(t *testing.T) {
		cfg := Defaults()
		delete(cfg.ModelProfiles.Cloud.Providers, "mistral")
		custom := p
		custom.TierOverrides = map[CostTier]string{CostPremium: "custom-model"}
		custom.ImageModel = "custom-vision"
		cfg.CloudProfiles = []CloudProfile{custom}
		cfg.ActiveCloudProfile = custom.Name
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := saveFixture(t, cfg, path); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		check(t, loaded)
		if len(loaded.CloudProfiles) != 1 {
			t.Fatalf("profiles = %+v", loaded.CloudProfiles)
		}
		if got := loaded.ModelProfiles.ResolveCloudModelForTier(loaded.CloudProfiles[0], TierVision); got != "custom-vision" {
			t.Fatalf("explicit image override = %q", got)
		}
		if got := loaded.ModelProfiles.ResolveCloudModelForTier(loaded.CloudProfiles[0], TierMostCapable); got != "custom-model" {
			t.Fatalf("explicit override = %q", got)
		}
	})
}
