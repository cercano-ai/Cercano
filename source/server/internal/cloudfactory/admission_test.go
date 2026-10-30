package cloudfactory

import (
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"context"
	"testing"
)

type requestFake struct{}

func (requestFake) CredentialSnapshot(context.Context, string) (secrets.CredentialSnapshot, error) {
	panic("unexpected credential access")
}
func (requestFake) AdmitAttempt(context.Context, string, string) error { panic("unexpected admission") }
func TestCoordinatedUnsupportedBeforeConstructor(t *testing.T) {
	// Explicit fake only; unsupported Bedrock must not enter AWS default chains.
	credentialtest.EnableHermetic(t)
	for _, p := range []config.CloudProfile{
		{Name: "fixture", Flavor: FlavorBedrock, Region: "us-east-1"},
		{Name: "fixture", Flavor: FlavorMessages, Route: RouteSubscription},
		{Name: "fixture", Flavor: FlavorMessages, Route: "unknown"},
		{Name: "fixture", Flavor: FlavorChatCompletions, Route: "unsupported"},
		{Name: "fixture", Flavor: FlavorResponses, Route: RouteChatGPT},
	} {
		if _, err := BuildCloudProvider(p, "", Options{Credentials: requestFake{}, AnthropicBaseTransport: &credentialtest.ForbiddenTransport{}, OpenAIBaseTransport: &credentialtest.ForbiddenTransport{}}); err == nil {
			t.Fatalf("allowed %s/%s", p.Flavor, p.Route)
		}
	}
}

func TestCoordinatedStaticMessagesConstruction(t *testing.T) {
	credentialtest.EnableHermetic(t)
	for _, route := range []string{"", "direct"} {
		p, err := BuildCloudProvider(config.CloudProfile{Name: "fixture", Flavor: FlavorMessages, Route: route}, "ignored", Options{Credentials: requestFake{}, AnthropicBaseTransport: &credentialtest.ForbiddenTransport{}})
		if err != nil || p == nil {
			t.Fatalf("route=%q error=%v", route, err)
		}
	}
}

func TestCoordinatedStaticChatConstruction(t *testing.T) {
	credentialtest.EnableHermetic(t)
	wire := &credentialtest.ForbiddenTransport{}
	for _, route := range []string{"", "direct", "chatgpt", "subscription", "unknown"} {
		p, err := BuildCloudProvider(config.CloudProfile{Name: "fixture", Flavor: FlavorChatCompletions, Route: route, Provider: "mistral"}, "ignored", Options{Credentials: requestFake{}, OpenAIBaseTransport: wire})
		if route == "" {
			if err != nil || p == nil {
				t.Fatalf("static: %v", err)
			}
		} else if err == nil {
			t.Fatalf("allowed route %q", route)
		}
	}
	if wire.Calls.Load() != 0 {
		t.Fatal("construction reached wire")
	}
}
