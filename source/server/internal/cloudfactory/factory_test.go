package cloudfactory

import (
	"cercano/source/server/internal/credentialtest"
	"context"
	"testing"

	"cercano/source/server/pkg/config"
)

func TestBuildMessagesProvider(t *testing.T) {
	credentialtest.Enable(t)
	p, err := BuildCloudProvider(config.CloudProfile{Name: "c", Flavor: "messages", Model: "claude-x"}, "sk")
	if err != nil || p == nil || p.Name() != "anthropic" {
		t.Fatalf("messages → %v, %v", p, err)
	}
}

func TestBuildChatCompletionsProvider(t *testing.T) {
	credentialtest.Enable(t)
	p, err := BuildCloudProvider(config.CloudProfile{Name: "o", Flavor: "chat_completions", Model: "gpt-4o"}, "sk")
	if err != nil || p == nil || p.Name() != "openai" {
		t.Fatalf("chat_completions → %v, %v", p, err)
	}
}

func TestBuildUnsupportedFlavor(t *testing.T) {
	credentialtest.Enable(t)
	if _, err := BuildCloudProvider(config.CloudProfile{Name: "c", Flavor: ""}, "sk"); err == nil {
		t.Error("empty flavor should error")
	}
}

func TestBuildChatCompletionsWithBackend(t *testing.T) {
	credentialtest.Enable(t)
	p, err := BuildCloudProvider(config.CloudProfile{
		Name: "g", Flavor: "chat_completions", Backend: "gemini", Model: "gemini-2.5-flash",
	}, "sk")
	if err != nil || p == nil || p.Name() != "openai" {
		t.Fatalf("chat_completions+backend → %v, %v", p, err)
	}
}

func TestBuildResponsesProvider(t *testing.T) {
	credentialtest.Enable(t)
	p, err := BuildCloudProvider(config.CloudProfile{Name: "r", Flavor: "responses", Model: "gpt-5"}, "sk")
	if err != nil || p == nil || p.Name() != "openai-responses" {
		t.Fatalf("responses → %v, %v", p, err)
	}
}

func TestBuildBedrockProvider(t *testing.T) {
	credentialtest.Enable(t)
	// Construction only: never consult any ambient credential provider.
	t.Setenv("AWS_ACCESS_KEY_ID", "synthetic-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "synthetic-secret-key")
	p, err := BuildCloudProvider(config.CloudProfile{Name: "b", Flavor: "bedrock", Region: "us-east-1", Model: "anthropic.claude-x"}, "")
	if err != nil || p == nil || p.Name() != "bedrock" {
		t.Fatalf("bedrock → %v, %v", p, err)
	}
}

func TestBuildBedrockMissingRegion(t *testing.T) {
	credentialtest.Enable(t)
	if _, err := BuildCloudProvider(config.CloudProfile{Name: "b", Flavor: "bedrock", Model: "m"}, ""); err == nil {
		t.Error("bedrock without a region should error")
	}
}

// stubAnthropicTokens is a fixed anthropic.TokenSource for the subscription
// route tests (satisfies the interface structurally).
type stubAnthropicTokens struct{}

func (stubAnthropicTokens) Token(ctx context.Context) (string, error) { return "tok", nil }

// The subscription route can't build without a token source — there is no
// static key to fall back on, so this must fail loudly rather than wire a dead
// provider (the "force re-auth" state for a not-yet-signed-in profile).
func TestBuildMessagesSubscriptionRequiresTokenSource(t *testing.T) {
	credentialtest.Enable(t)
	if _, err := BuildCloudProvider(
		config.CloudProfile{Name: "c", Flavor: "messages", Route: RouteSubscription, Model: "claude-x"}, "",
	); err == nil {
		t.Error("subscription route without a token source should error")
	}
}

func TestBuildMessagesSubscriptionWithTokenSource(t *testing.T) {
	credentialtest.Enable(t)
	p, err := BuildCloudProvider(
		config.CloudProfile{Name: "c", Flavor: "messages", Route: RouteSubscription, Model: "claude-x"},
		"",
		Options{AnthropicTokenSource: stubAnthropicTokens{}},
	)
	if err != nil || p == nil || p.Name() != "anthropic" {
		t.Fatalf("subscription with token source → %v, %v", p, err)
	}
}
