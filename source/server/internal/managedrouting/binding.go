package managedrouting

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/inference/profilechain"
	"cercano/source/server/internal/modelpolicy"
	"cercano/source/server/pkg/config"
	v1 "github.com/bryancostanich/Cercano/source/enterpriseapi/v1"
)

// BindRoute reuses credentials only from a configured profile with the same
// physical provider and endpoint. A model policy cannot redirect a saved key to
// another service. It never builds the profile's personal backup/account cycle.
func BindRoute(ctx context.Context, c config.Config, route v1.Route, destination config.Destination, open inference.Provider, build func(config.CloudProfile) (inference.Provider, error)) (inference.Candidate, error) {
	localSource := func() (string, error) {
		if len(route.AllowedSourceConnectionIDs) == 0 {
			return "", nil
		}
		if c.LocalSourceConnectionID == "" {
			return "", fmt.Errorf("local route %q needs this host's configured physical source ID", route.ID)
		}
		for _, allowed := range route.AllowedSourceConnectionIDs {
			if allowed == c.LocalSourceConnectionID {
				return allowed, nil
			}
		}
		return "", fmt.Errorf("local route %q does not authorize this host's physical source ID", route.ID)
	}
	if route.Provider == "ollama" {
		endpoint := strings.TrimRight(c.OllamaURL, "/")
		parsed, _ := url.Parse(endpoint)
		if open == nil || c.OpenRuntime != "ollama" || endpoint != route.Endpoint || modelpolicy.Placement(parsed) != route.Placement {
			return inference.Candidate{}, fmt.Errorf("approved Ollama route %q does not match the configured runtime", route.ID)
		}
		sourceID, err := localSource()
		if err != nil {
			return inference.Candidate{}, err
		}
		return inference.Candidate{Provider: open, IsCloud: route.Placement == "external", SourceConnectionID: sourceID}, nil
	}
	if route.Placement == "local" {
		if open == nil || c.OpenRuntime != route.Provider {
			return inference.Candidate{}, fmt.Errorf("approved local route %q does not match the configured runtime", route.ID)
		}
		// Supervised runtime ports are resolved when the model is prepared. The
		// physical-request gate checks that exact endpoint against the current policy.
		sourceID, err := localSource()
		if err != nil {
			return inference.Candidate{}, err
		}
		return inference.Candidate{Provider: open, SourceConnectionID: sourceID}, nil
	}
	preferred, _ := c.DestinationProfiles(destination)
	profiles := append([]config.CloudProfile(nil), c.CloudProfiles...)
	for i, p := range profiles {
		if p.Name == preferred {
			profiles[0], profiles[i] = profiles[i], profiles[0]
			break
		}
	}
	for _, profile := range profiles {
		provider, endpoint, err := cloudfactory.PhysicalEndpoint(ctx, profile)
		if err != nil || provider != route.Provider || endpoint != route.Endpoint {
			continue
		}
		// An ID is reportable only when the signed route authorizes that exact
		// physical connection. Legacy routes carry no attributable source.
		connectionID := ""
		if len(route.AllowedSourceConnectionIDs) > 0 {
			for _, allowed := range route.AllowedSourceConnectionIDs {
				if profile.SourceConnectionID != "" && profile.SourceConnectionID == allowed {
					connectionID = allowed
					break
				}
			}
			if connectionID == "" {
				if profile.Name == preferred {
					return inference.Candidate{}, fmt.Errorf("preferred profile %q has no approved physical source on route %q", preferred, route.ID)
				}
				continue
			}
		}
		if build == nil {
			return inference.Candidate{}, fmt.Errorf("managed cloud provider builder unavailable")
		}
		profile.Model = route.Model
		raw, err := build(profile)
		if raw != nil {
			raw = profilechain.WithRoute(raw, profile.Name, string(destination))
		}
		return inference.Candidate{Provider: raw, Profile: profile.Name, IsCloud: true, SourceConnectionID: connectionID}, err
	}
	return inference.Candidate{}, fmt.Errorf("approved route %q needs a configured %s profile for %s", route.ID, route.Provider, route.Endpoint)
}
