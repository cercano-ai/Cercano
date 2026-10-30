package providers

import (
	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/credentialtest"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"testing"
)

func TestCoordinatedMessagesHostGraph(t *testing.T) {
	credentialtest.EnableHermetic(t)
	authority := &hostCredentialFake{Store: secrets.NewMemory()}
	if err := authority.Store.Set("fixture", "fake-static-key"); err != nil {
		t.Fatal(err)
	}
	s := &service{cfgSvc: cfgsvc.New("", config.Defaults(), authority), router: &recordingRouter{}}
	calls := 0
	s.SetCloudBuilder(func(p config.CloudProfile, key string, opts ...cloudfactory.Options) (inference.Provider, error) {
		calls++
		if len(opts) != 1 || opts[0].Credentials == nil || opts[0].Credentials == authority || opts[0].TokenSource != nil || opts[0].AnthropicTokenSource != nil {
			t.Fatal("missing facade or unexpected subscription source")
		}
		opts[0].AnthropicBaseTransport = &credentialtest.ForbiddenTransport{}
		return cloudfactory.BuildCloudProvider(p, key, opts...)
	})
	for _, route := range []string{"", "direct"} {
		if _, err := s.buildProfile(config.CloudProfile{Name: "fixture", Flavor: cloudfactory.FlavorMessages, Route: route}); err != nil {
			t.Fatal(err)
		}
	}
	for _, route := range []string{"meridian", "unknown"} {
		if _, err := s.buildProfile(config.CloudProfile{Name: "fixture", Flavor: cloudfactory.FlavorMessages, Route: route}); err == nil {
			t.Fatal("unsupported route allowed")
		}
	}
	if calls != 2 {
		t.Fatalf("constructors=%d", calls)
	}
}
