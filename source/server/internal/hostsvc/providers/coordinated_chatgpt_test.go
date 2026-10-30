package providers

import (
	"context"
	"errors"
	"testing"

	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/credentialtest"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm/responses"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

// Entire authority is memory-only; no coordinated filesystem/native constructor.
type hostCredentialFake struct {
	secrets.Store
	deleted bool
}

func (*hostCredentialFake) IsCoordinated() bool { return true }
func (*hostCredentialFake) CredentialRevision(context.Context, string) (string, error) {
	return "revision", nil
}
func (*hostCredentialFake) SetAtRevision(context.Context, string, string, string) error {
	return errors.New("unexpected write")
}
func (f *hostCredentialFake) Delete(string) error { f.deleted = true; return nil }
func (f *hostCredentialFake) CredentialSnapshot(_ context.Context, name string) (secrets.CredentialSnapshot, error) {
	if name != "subscription-profile" {
		return secrets.CredentialSnapshot{}, errors.New("wrong profile")
	}
	if f.deleted {
		return secrets.CredentialSnapshot{}, errors.New("logged out")
	}
	return secrets.CredentialSnapshot{Payload: `{"access":"fake-access","account_id":"fake-account","expires_at":"2100-01-01T00:00:00Z"}`, Revision: "revision"}, nil
}
func (f *hostCredentialFake) AdmitAttempt(ctx context.Context, name, revision string) error {
	if revision != "revision" {
		return errors.New("wrong revision")
	}
	_, err := f.CredentialSnapshot(ctx, name)
	return err
}

func TestCoordinatedChatGPTHostGraph(t *testing.T) {
	credentialtest.EnableHermetic(t)
	authority := &hostCredentialFake{Store: secrets.NewMemory()}
	prof := config.CloudProfile{Name: "subscription-profile", Flavor: cloudfactory.FlavorResponses, Route: cloudfactory.RouteChatGPT, Model: "fixture"}
	c := config.Defaults()
	c.LocusMode = "cloud_only"
	c.ActiveCloudProfile = prof.Name
	c.CloudProfiles = []config.CloudProfile{prof}
	s := &service{cfgSvc: cfgsvc.New("", c, authority), router: &recordingRouter{}}
	var capability secrets.RequestCredentials
	var subscription responses.SubscriptionCredentials
	calls := 0
	s.SetCloudBuilder(func(got config.CloudProfile, key string, opts ...cloudfactory.Options) (inference.Provider, error) {
		calls++
		if got.Name != prof.Name || got.Flavor != prof.Flavor || got.Route != prof.Route || key != "" {
			t.Fatal("profile changed or raw credential fallback")
		}
		capability = opts[0].Credentials
		if capability == authority {
			t.Fatal("authority bypassed Service/read-only facade")
		}
		if _, _, err := opts[0].TokenSource.Token(context.Background()); err == nil {
			t.Fatal("legacy raw token fallback enabled")
		}
		var ok bool
		subscription, ok = opts[0].TokenSource.(responses.SubscriptionCredentials)
		if capability == nil || !ok || subscription.CredentialProfile() != prof.Name {
			t.Fatal("missing coordinated capability/profile")
		}
		// Exercise the real factory construction, but never enable its external transport.
		return cloudfactory.BuildCloudProvider(got, key, opts...)
	})
	if err := s.Rebuild(); err != nil {
		t.Fatalf("supported coordinated ChatGPT unavailable: %v", err)
	}
	if s.Cloud() == nil || calls == 0 {
		t.Fatal("graph not published")
	}
	ctx := context.Background()
	snap, err := subscription.SubscriptionSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := capability.AdmitAttempt(ctx, prof.Name, snap.Revision); err != nil {
		t.Fatal(err)
	}
	if err := s.cfgSvc.Credentials().Delete(prof.Name); err != nil {
		t.Fatal(err)
	}
	if err := capability.AdmitAttempt(ctx, prof.Name, snap.Revision); err == nil {
		t.Fatal("logout did not fence captured capability")
	}
	if _, err := subscription.SubscriptionSnapshot(ctx); err == nil {
		t.Fatal("logout did not fence subscription source")
	}
	before := calls
	for _, unsupported := range []config.CloudProfile{
		{Flavor: cloudfactory.FlavorMessages, Route: "meridian"},
		{Flavor: cloudfactory.FlavorResponses, Route: "other"},
		{Flavor: cloudfactory.FlavorBedrock},
	} {
		if _, err := s.buildProfile(unsupported); err == nil {
			t.Fatalf("allowed unsupported profile: %+v", unsupported)
		}
	}
	// Supported static routes still require a named profile/key, independently of route admission.
	for _, flavor := range []string{cloudfactory.FlavorMessages, cloudfactory.FlavorChatCompletions} {
		if _, err := s.buildProfile(config.CloudProfile{Flavor: flavor}); err == nil {
			t.Fatal("missing key accepted")
		}
	}
	if calls != before {
		t.Fatal("unsupported route reached constructor")
	}
}
