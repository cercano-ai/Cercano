package server

import (
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/inference/profilechain"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/modelevidence"
	"cercano/source/server/internal/modelmetadata"
	"cercano/source/server/pkg/config"
	"context"
	"testing"
)

func TestImageInheritanceSnapshotIncludesProviderRecommendations(t *testing.T) {
	s, _ := newTestServer()
	c := config.Config{ActiveCloudProfile: "p", ModelProfiles: config.Defaults().ModelProfiles, CloudProfiles: []config.CloudProfile{{Name: "p", Provider: "anthropic", Flavor: "messages", TierOverrides: map[config.CostTier]string{config.CostEconomy: "custom-text", config.CostStandard: "custom-text", config.CostPremium: "custom-text"}}}}
	snapshot := s.turnModelEvidence(t.Context(), c)
	p := c.CloudProfiles[0]
	candidates := c.ModelProfiles.CloudImageCandidates(p, config.TierEveryday)
	if len(candidates) < 2 {
		t.Fatalf("no provider fallback: %v", candidates)
	}
	for _, model := range candidates[1:] {
		evidence, ok := snapshot.Lookup(modelevidence.IdentityFor(p, model))
		if !ok || evidence.Vision != modelmetadata.VisionSupported {
			t.Fatalf("worker missing image fallback evidence for %q: %+v", model, evidence)
		}
	}
	if p.ImageModel != "" || p.TierOverrides[config.CostStandard] != "custom-text" {
		t.Fatal("resolution changed explicit choices")
	}
}

// The transport records calls only; capability decisions use the real resolver.
type subscriptionImageTransport struct{ calls []llm.ChatRequest }

func (*subscriptionImageTransport) Name() string { return "fixture" }
func (*subscriptionImageTransport) Capabilities() inference.Capabilities {
	return inference.Capabilities{SupportsVision: true}
}
func (p *subscriptionImageTransport) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	p.calls = append(p.calls, req)
	return llm.ChatResponse{Model: req.Model}, nil
}
func (*subscriptionImageTransport) StreamChat(context.Context, llm.ChatRequest) (llm.StreamReader, error) {
	panic("unused")
}

func TestImageInheritanceDefaultSubscriptionProfiles(t *testing.T) {
	for _, tc := range []struct{ name, flavor, route, model string }{
		{"chatgpt", "responses", "chatgpt", "gpt-6-astra"},
		{"claude", "messages", "subscription", "claude-fable-5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestServer()
			// No model or task overrides: default Premium chat must be inherited.
			profile := config.CloudProfile{Name: "account-key", Flavor: tc.flavor, Route: tc.route}
			c := config.Config{ActiveCloudProfile: profile.Name, ModelProfiles: config.Defaults().ModelProfiles, CloudProfiles: []config.CloudProfile{profile}}
			snapshot := s.turnModelEvidence(t.Context(), c)
			raw := &subscriptionImageTransport{}
			chain, err := profilechain.Build(c, config.DestinationPrimary, func(p config.CloudProfile) (inference.Provider, error) {
				evidence := func(model string) modelmetadata.Evidence {
					ev, ok := snapshot.Lookup(modelevidence.IdentityFor(p, model))
					if !ok {
						t.Errorf("missing evidence for %s", model)
					}
					return ev
				}
				return profilechain.GuardVision(raw, func(model string) bool { return evidence(model).Vision == modelmetadata.VisionSupported }, evidence), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			target := inference.TargetForCall(chain, inference.Call{Tier: "vision"})
			if target.Model != tc.model || !target.SupportsVision {
				t.Fatalf("image target=%+v", target)
			}
			_, err = chain.Chat(t.Context(), inference.Call{Tier: "vision", Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockImage, ImageData: "fixture"}}}}})
			if err != nil || len(raw.calls) != 1 || raw.calls[0].Model != tc.model {
				t.Fatalf("calls=%+v err=%v", raw.calls, err)
			}
			if c.CloudProfiles[0].ImageModel != "" || c.CloudProfiles[0].Provider != "" || c.CloudProfiles[0].Name != profile.Name {
				t.Fatal("resolution changed profile")
			}
		})
	}
}
