package main

import (
	"context"
	"fmt"
	"strings"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/anthropicauth"
	"cercano/source/server/internal/chatgptauth"
	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/hostsvc/credentials"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

// validateDevelopmentCloudProfiles runs before credential admission can turn a
// construction error into a temporarily blocked (but otherwise usable) route.
func validateDevelopmentCloudProfiles(cfg config.Config) error {
	seen := make(map[string]bool)
	for _, p := range cfg.CloudProfiles {
		if p.Name == "" || seen[p.Name] {
			return fmt.Errorf("prepared cloud profiles require unique nonempty names")
		}
		seen[p.Name] = true
		if err := cloudfactory.ValidateFlavor(p.Flavor); err != nil {
			return fmt.Errorf("prepared cloud profile %q: %w", p.Name, err)
		}
	}
	if cfg.ActiveCloudProfile != "" && !seen[cfg.ActiveCloudProfile] {
		return fmt.Errorf("active prepared cloud profile %q does not exist", cfg.ActiveCloudProfile)
	}
	return nil
}

// developmentCloudFactory resolves the old router callback against the frozen
// configured profiles, never the callback's loose API-key/endpoint arguments.
// In particular, a configured responses/Bedrock/subscription route must not be
// silently reconstructed as a legacy chat-completions provider.
func developmentCloudFactory(cfg config.Config, store secrets.Store) agent.CloudFactory {
	snapshot := cfg.WithSharedReadOnly()
	frozen := secrets.Guard(store, secrets.Options{ReadOnly: true})
	creds := credentials.New(frozen, secrets.Options{ReadOnly: true})
	return func(_ context.Context, provider, model, _, _ string) (agent.TurnRunner, error) {
		var profile config.CloudProfile
		found := false
		// Prefer an exact profile identity, then the active provider identity.
		for _, p := range snapshot.CloudProfiles {
			if p.Name == provider {
				profile, found = p, true
				break
			}
		}
		if !found {
			if p, ok := snapshot.Profile(snapshot.ActiveCloudProfile); ok && strings.EqualFold(p.Provider, provider) {
				profile, found = p, true
			}
		}
		if !found {
			return nil, fmt.Errorf("production development host: cloud route %q is not a configured profile", provider)
		}
		if model != "" {
			profile.Model = model
		}
		options := cloudfactory.Options{}
		key := ""
		switch {
		case profile.Flavor == cloudfactory.FlavorResponses && profile.Route == cloudfactory.RouteChatGPT:
			options.TokenSource = creds.ChatGPT(profile.Name, chatgptauth.Flow{})
		case profile.Flavor == cloudfactory.FlavorMessages && profile.Route == cloudfactory.RouteSubscription:
			options.AnthropicTokenSource = creds.Anthropic(profile.Name, anthropicauth.Flow{})
		case profile.Flavor != cloudfactory.FlavorBedrock:
			var err error
			key, err = frozen.Get(profile.Name)
			if err := cloudfactory.ValidateStaticCredential(profile, key, err); err != nil {
				return nil, err
			}
		}
		p, err := cloudfactory.BuildCloudProvider(profile, key, options)
		if err != nil {
			return nil, err
		}
		return agent.InferenceTurnRunner(p, profile.Model), nil
	}
}
