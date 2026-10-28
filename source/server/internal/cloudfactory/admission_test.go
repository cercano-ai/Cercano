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
	credentialtest.Enable(t)
	for _, p := range []config.CloudProfile{
		{Name: "fixture", Flavor: FlavorBedrock, Region: "us-east-1"},
		{Name: "fixture", Flavor: FlavorMessages},
		{Name: "fixture", Flavor: FlavorChatCompletions},
		{Name: "fixture", Flavor: FlavorResponses, Route: RouteChatGPT},
	} {
		if _, err := BuildCloudProvider(p, "", Options{Credentials: requestFake{}}); err == nil {
			t.Fatalf("allowed %s/%s", p.Flavor, p.Route)
		}
	}
}
