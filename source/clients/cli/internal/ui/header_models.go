package ui

import (
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
)

// configuredHeaderModels names destinations, not quality slots or accounts.
// Chat and dispatch supply each destination's default quality, as in profilechain.
// Explicit redirects preserve that quality; missing bindings stay missing.
func configuredHeaderModels(cfg *agentclient.Config, view *agentclient.CloudProvidersView) (string, string) {
	c := config.Config{ActiveCloudProfile: view.Active, TaskAssignments: map[config.Task]config.TaskAssignment{}}
	if a := view.Assignments; a != nil {
		c.ActiveCloudProfile, c.SecondaryCloudProfile = a.Primary, a.Secondary
		c.SecondaryRedirect, c.LocalRedirect = config.Destination(a.SecondaryRedirect), config.Destination(a.LocalRedirect)
		for task, v := range a.Tasks {
			c.TaskAssignments[config.Task(task)] = config.TaskAssignment{Destination: config.Destination(v.Destination), Quality: config.CostTier(v.Quality)}
		}
	}
	profiles := append([]agentclient.CloudProfileInfo(nil), view.CustomProfiles...)
	for _, provider := range view.Providers {
		profiles = append(profiles, provider.Profiles...)
	}
	resolve := func(d config.Destination, task config.Task) string {
		quality := c.TaskAssignment(task).Quality
		resolved, err := c.ResolveDestination(d)
		if err != nil {
			return ""
		}
		if resolved == config.DestinationLocal {
			if model := cfg.ModelTiers[activeTierKey(cfg, string(quality.CapabilityTier()))]; model != "" {
				return model
			}
			return cfg.OpenModel
		}
		name, _ := c.DestinationProfiles(resolved)
		for _, p := range profiles {
			if p.Name == name {
				return p.EffectiveQualityModels[string(quality)]
			}
		}
		return ""
	}
	return resolve(config.DestinationPrimary, config.TaskChat), resolve(config.DestinationSecondary, config.TaskDispatch)
}
