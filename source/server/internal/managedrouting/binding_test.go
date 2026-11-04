package managedrouting

import (
	"context"
	"testing"

	"cercano/source/server/internal/inference"
	"cercano/source/server/pkg/config"
	v1 "github.com/bryancostanich/Cercano/source/enterpriseapi/v1"
)

func TestManagedBindingDoesNotRedirectProviderCredentials(t *testing.T) {
	c := config.Config{ActiveCloudProfile: "personal", SecondaryCloudProfile: "work", CloudProfiles: []config.CloudProfile{
		{Name: "personal", Flavor: "chat_completions", BaseURL: "https://models.example/v1", Model: "personal-model"},
		{Name: "work", Flavor: "chat_completions", BaseURL: "https://models.example/v1", Model: "work-model"},
	}}
	route := v1.Route{ID: "approved", Provider: "openai", Endpoint: "https://models.example/v1", Model: "admin-model", Placement: "external"}
	var built []config.CloudProfile
	build := func(p config.CloudProfile) (inference.Provider, error) {
		built = append(built, p)
		return &routeFixture{route: route}, nil
	}
	candidate, err := BindRoute(context.Background(), c, route, config.DestinationSecondary, nil, build)
	if err != nil || candidate.Profile != "work" || len(built) != 1 || built[0].Model != "admin-model" {
		t.Fatalf("candidate=%+v built=%+v err=%v", candidate, built, err)
	}
	if c.CloudProfiles[1].Model != "work-model" {
		t.Fatal("managed binding overwrote personal configuration")
	}
	for _, change := range []string{"provider", "endpoint"} {
		other := route
		if change == "provider" {
			other.Provider = "anthropic"
		} else {
			other.Endpoint = "https://unconfigured.example/v1"
		}
		if _, err := BindRoute(context.Background(), c, other, config.DestinationPrimary, nil, build); err == nil {
			t.Fatal("bound a key to an unconfigured identity")
		}
	}
	if len(built) != 1 {
		t.Fatal("unapproved endpoint reached credential builder")
	}
}

func TestManagedBindingUsesSignedPhysicalConnection(t *testing.T) {
	c := config.Config{ActiveCloudProfile: "api-key", SecondaryCloudProfile: "subscription", CloudProfiles: []config.CloudProfile{
		{Name: "api-key", SourceConnectionID: "source-api", Flavor: "chat_completions", BaseURL: "https://models.example/v1"},
		{Name: "subscription", SourceConnectionID: "source-sub", Flavor: "chat_completions", BaseURL: "https://models.example/v1"},
	}}
	route := v1.Route{ID: "shared", Provider: "openai", Endpoint: "https://models.example/v1", Model: "model", Placement: "external", AllowedSourceConnectionIDs: []string{"source-api", "source-sub"}}
	build := func(p config.CloudProfile) (inference.Provider, error) { return &routeFixture{route: route}, nil }
	primary, err := BindRoute(t.Context(), c, route, config.DestinationPrimary, nil, build)
	if err != nil || primary.SourceConnectionID != "source-api" || primary.Profile != "api-key" {
		t.Fatalf("primary %+v: %v", primary, err)
	}
	secondary, err := BindRoute(t.Context(), c, route, config.DestinationSecondary, nil, build)
	if err != nil || secondary.SourceConnectionID != "source-sub" || secondary.Profile != "subscription" {
		t.Fatalf("secondary %+v: %v", secondary, err)
	}
	route.AllowedSourceConnectionIDs = []string{"source-api"}
	if _, err := BindRoute(t.Context(), c, route, config.DestinationSecondary, nil, build); err == nil {
		t.Fatal("preferred unapproved source silently switched to a different credential")
	}
	route.AllowedSourceConnectionIDs = []string{"unconfigured"}
	if _, err := BindRoute(t.Context(), c, route, config.DestinationPrimary, nil, build); err == nil {
		t.Fatal("accepted an unconfigured source")
	}
	route.AllowedSourceConnectionIDs = nil
	legacy, err := BindRoute(t.Context(), c, route, config.DestinationPrimary, nil, build)
	if err != nil || legacy.SourceConnectionID != "" {
		t.Fatalf("legacy route fabricated source: %+v %v", legacy, err)
	}
}
func TestManagedOllamaBindingPreservesPhysicalPlacement(t *testing.T) {
	var calls []string
	for _, tc := range []struct{ endpoint, placement string }{{"http://127.0.0.1:11434", "local"}, {"https://ollama.example", "external"}} {
		route := v1.Route{ID: "ollama", Provider: "ollama", Endpoint: tc.endpoint, Model: "approved", Placement: tc.placement}
		raw := &routeFixture{route: route, calls: &calls}
		c := config.Config{OpenRuntime: "ollama", OllamaURL: tc.endpoint}
		candidate, err := BindRoute(context.Background(), c, route, config.DestinationPrimary, raw, nil)
		if err != nil || candidate.IsCloud != (tc.placement == "external") {
			t.Fatalf("placement lost: %+v %v", candidate, err)
		}
		route.Endpoint = "https://other.example"
		if _, err := BindRoute(context.Background(), c, route, config.DestinationPrimary, raw, nil); err == nil {
			t.Fatal("runtime endpoint mismatch accepted")
		}
	}
}

func TestLocalRoutesBindOneSignedPhysicalSource(t *testing.T) {
	for _, provider := range []string{"ollama", "llama.cpp"} {
		route := v1.Route{ID: "local", Provider: provider, Endpoint: "http://127.0.0.1:11434", Model: "approved", Placement: "local", AllowedSourceConnectionIDs: []string{"workstation"}}
		c := config.Config{OpenRuntime: provider, OllamaURL: route.Endpoint, LocalSourceConnectionID: "workstation"}
		raw := &routeFixture{route: route}
		got, err := BindRoute(t.Context(), c, route, config.DestinationPrimary, raw, nil)
		if err != nil || got.SourceConnectionID != "workstation" {
			t.Fatalf("%s lost source: %+v %v", provider, got, err)
		}
		route.AllowedSourceConnectionIDs = []string{"other", "workstation"}
		got, err = BindRoute(t.Context(), c, route, config.DestinationPrimary, raw, nil)
		if err != nil || got.SourceConnectionID != "workstation" {
			t.Fatalf("%s failed exact host source selection: %+v %v", provider, got, err)
		}
		c.LocalSourceConnectionID = ""
		if _, err := BindRoute(t.Context(), c, route, config.DestinationPrimary, raw, nil); err == nil {
			t.Fatalf("%s guessed host source", provider)
		}
		c.LocalSourceConnectionID = "foreign"
		if _, err := BindRoute(t.Context(), c, route, config.DestinationPrimary, raw, nil); err == nil {
			t.Fatalf("%s accepted foreign host source", provider)
		}
	}
}
