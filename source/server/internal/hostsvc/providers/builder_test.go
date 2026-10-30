package providers

import (
	"context"
	"errors"
	"testing"

	"cercano/source/server/internal/cloudfactory"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

func TestCloudBuilderInvocationAndCredentialBoundary(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	// Enabled only in this isolated memory-store/fake-builder unit scope.
	// Subscription token sources remain read-only and the fake never invokes them.
	prof := config.CloudProfile{Name: "fixture", Flavor: cloudfactory.FlavorMessages, Route: cloudfactory.RouteSubscription, Backend: "anthropic", BaseURL: "http://127.0.0.1:1/v1", Model: "fixture"}
	calls := [2]int{}
	services := [2]*service{}
	failed := errors.New("fixture builder failure")
	for i := range services {
		c := config.Defaults()
		c.LocusMode = "cloud_only"
		c.ActiveCloudProfile = prof.Name
		c.CloudProfiles = []config.CloudProfile{prof}
		s := &service{cfgSvc: cfgsvc.New("", c, secrets.NewMemory()), router: &recordingRouter{}}
		s.SetCloudBuilder(func(config.CloudProfile, string, ...cloudfactory.Options) (inference.Provider, error) {
			calls[i]++
			if i == 0 {
				return nil, failed
			}
			return &labelProvider{label: "second"}, nil
		})
		services[i] = s
	}
	if err := services[0].Rebuild(); err == nil {
		t.Fatal("builder failure suppressed")
	}
	if services[0].Cloud() != nil {
		t.Fatal("failed builder published provider")
	}
	if err := services[1].Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := services[0].Rebuild(); err == nil {
		t.Fatal("second failure suppressed")
	}
	result, err := services[1].Cloud().Chat(context.Background(), inference.Call{Model: "fixture"})
	if err != nil || result.Model != "second:fixture" || calls[0] == 0 || calls[1] == 0 {
		t.Fatalf("independent graph: %+v %v calls=%v", result, err, calls)
	}
	before := calls
	required := prof
	required.Flavor = cloudfactory.FlavorChatCompletions
	required.Route = ""
	required.Provider = "deepinfra"
	required.BaseURL = "https://api.deepinfra.com/v1/openai"
	if _, err := services[1].buildProfile(required); err == nil || errors.Is(err, secrets.ErrDisabled) {
		t.Fatalf("enabled missing-key must fail independently of disabled policy: %v", err)
	}
	if calls != before {
		t.Fatal("builder ran before credential validation")
	}
}
