package ui

import (
	"cercano/source/server/pkg/agentclient"
	"testing"
)

func TestConfiguredHeaderModels(t *testing.T) {
	for _, tt := range []struct {
		name, secondary, redirect, localRedirect, quality, want string
	}{
		{name: "unconfigured"},
		{name: "missing profile", secondary: "deleted"},
		{name: "independent account", secondary: "second", want: "second-standard"},
		{name: "explicit primary redirect", redirect: "primary", want: "primary-standard"},
		{name: "quality is independent of destination", redirect: "primary", quality: "economy", want: "primary-economy"},
		{name: "local redirect", redirect: "local", want: "local-standard"},
		{name: "transitive redirect", redirect: "local", localRedirect: "primary", want: "primary-standard"},
		{name: "redirect cycle", redirect: "local", localRedirect: "secondary"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			view := agentclient.CloudProvidersView{Active: "stale", Assignments: &agentclient.RoutingAssignments{Primary: "first", Secondary: tt.secondary, SecondaryRedirect: tt.redirect, LocalRedirect: tt.localRedirect, Tasks: map[string]agentclient.TaskAssignment{"dispatch": {Quality: tt.quality}}}, CustomProfiles: []agentclient.CloudProfileInfo{
				{Name: "first", EffectiveQualityModels: map[string]string{"premium": "primary-premium", "standard": "primary-standard", "economy": "primary-economy"}},
				{Name: "second", EffectiveQualityModels: map[string]string{"standard": "second-standard"}},
			}}
			cfg := &agentclient.Config{OpenModel: "local-default", OpenRuntime: "llama_server", ModelTiers: map[string]string{"llama_server.everyday": "local-standard"}}
			primary, secondary := configuredHeaderModels(cfg, &view)
			if tt.name != "redirect cycle" && primary != "primary-premium" {
				t.Fatalf("primary=%q", primary)
			}
			if secondary != tt.want {
				t.Fatalf("secondary=%q want %q", secondary, tt.want)
			}
		})
	}
}
