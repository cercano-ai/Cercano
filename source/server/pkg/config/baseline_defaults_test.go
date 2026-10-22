package config

import "testing"

// Pins effective semantics to approved v0.25.0 (58befd2801dd6f3d53438e8c8bfc74d937a9e811).
func TestBaselineProviderDefaults(t *testing.T) {
	m := Defaults().ModelProfiles
	for vendor, want := range map[string][3]string{
		"openai":         {"gpt-6-luna", "gpt-6.1-sol", "gpt-6-astra"},
		"openai-chatgpt": {"gpt-6-luna", "gpt-6.1-sol", "gpt-6-astra"},
		"mistral":        {"mistral-small-latest", "mistral-large-4", "zai-glm-5-3"},
		"cerebras":       {"gpt-oss-120b", "", ""},
	} {
		t.Run(vendor, func(t *testing.T) {
			if _, ok := m.Cloud.Providers[vendor]; !ok {
				t.Error("missing provider table")
			}
			for i, cost := range []CostTier{CostEconomy, CostStandard, CostPremium} {
				got, ok := m.ResolveCloud(vendor, cost)
				if got != want[i] || ok != (want[i] != "") {
					t.Errorf("%s = %q, %v; want %q", cost, got, ok, want[i])
				}
				tier := []Tier{TierFastLightText, TierEveryday, TierMostCapable}[i]
				p := CloudProfile{Provider: vendor, Model: "ignored-legacy", ModelPinned: true}
				if got := m.ResolveCloudModelForTier(p, tier); got != want[i] {
					t.Errorf("%s resolution = %q, want %q", cost, got, want[i])
				}
				p.TierOverrides = map[CostTier]string{cost: "explicit-text"}
				if got := m.ResolveCloudModelForTier(p, tier); got != "explicit-text" {
					t.Errorf("override = %q", got)
				}
			}
		})
	}
}

func TestBaselineProviderInferenceAndVision(t *testing.T) {
	m := Defaults().ModelProfiles
	for _, tc := range []struct {
		name         string
		p            CloudProfile
		text, vision string
	}{
		{"mistral-host", CloudProfile{Flavor: "chat_completions", BaseURL: "https://api.mistral.ai/v1"}, "mistral-small-latest", "mistral-large-4"},
		{"mistral-case-port", CloudProfile{Flavor: "chat_completions", BaseURL: "https://API.MISTRAL.AI:443/v1"}, "mistral-small-latest", "mistral-large-4"},
		{"mistral-explicit", CloudProfile{Provider: "mistral"}, "mistral-small-latest", "mistral-large-4"},
		{"cerebras-host", CloudProfile{Flavor: "chat_completions", BaseURL: "https://api.cerebras.ai/v1"}, "gpt-oss-120b", ""},
		{"chatgpt-route", CloudProfile{Flavor: "responses", Route: "chatgpt"}, "gpt-6-luna", ""},
		{"explicit-provider-wins", CloudProfile{Provider: "mistral", Flavor: "responses", Route: "chatgpt"}, "mistral-small-latest", "mistral-large-4"},
		{"backend-wins", CloudProfile{Flavor: "chat_completions", Backend: "openai", BaseURL: "https://api.mistral.ai/v1"}, "gpt-6-luna", ""},
		{"not-mistral-host", CloudProfile{Flavor: "chat_completions", BaseURL: "https://api.mistral.ai.invalid/v1"}, "gpt-6-luna", ""},
		{"userinfo-not-inferred", CloudProfile{Flavor: "chat_completions", BaseURL: "https://user@api.mistral.ai/v1"}, "gpt-6-luna", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.ResolveCloudModelForTier(tc.p, TierFastLightText); got != tc.text {
				t.Errorf("text = %q, want %q", got, tc.text)
			}
			tc.p.TierOverrides = map[CostTier]string{CostEconomy: "text-only", CostStandard: "text-only", CostPremium: "text-only"}
			if got := m.ResolveCloudModelForTier(tc.p, TierVision); got != tc.vision {
				t.Errorf("vision = %q, want %q", got, tc.vision)
			}
			tc.p.ImageModel = "explicit-image"
			if got := m.ResolveCloudModelForTier(tc.p, TierVision); got != "explicit-image" {
				t.Errorf("image override = %q", got)
			}
		})
	}
}
