package providers

import (
	"errors"
	"strings"
	"testing"

	"cercano/source/server/internal/cloudfactory"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

func TestBuilderCredentialAdmission(t *testing.T) {
	for _, setting := range []string{"1", "", "malformed"} {
		t.Run("policy="+setting, func(t *testing.T) {
			t.Setenv(secrets.DisableEnv, setting)
			for _, flavor := range []string{cloudfactory.FlavorMessages, cloudfactory.FlavorResponses, cloudfactory.FlavorBedrock, cloudfactory.FlavorChatCompletions, "invalid"} {
				calls := 0
				s := &service{cfgSvc: cfgsvc.New("", config.Defaults(), secrets.NewMemory())}
				s.SetCloudBuilder(func(config.CloudProfile, string, ...cloudfactory.Options) (inference.Provider, error) {
					calls++
					return &labelProvider{}, nil
				})
				p := config.CloudProfile{Name: "fixture", Flavor: flavor, Route: cloudfactory.RouteSubscription}
				if flavor == cloudfactory.FlavorResponses {
					p.Route = cloudfactory.RouteChatGPT
				}
				_, err := s.buildProfile(p)
				if flavor == "invalid" {
					if err == nil || !strings.Contains(err.Error(), "not yet supported") {
						t.Errorf("static precedence: %v", err)
					}
				} else if !errors.Is(err, secrets.ErrDisabled) {
					t.Errorf("%s: expected policy refusal, got %v", flavor, err)
				}
				if calls != 0 {
					t.Errorf("%s: builder calls=%d; want zero", flavor, calls)
				}
			}
		})
	}
}
