package profilechain

import (
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/pkg/config"
	"context"
	"testing"
)

func TestImageInheritsConfirmedChatModel(t *testing.T) {
	for _, destination := range []config.Destination{config.DestinationPrimary, config.DestinationSecondary} {
		t.Run(string(destination), func(t *testing.T) {
			// These fixtures exercise Standard chat inheritance, not the Premium product default.
			c := config.Config{TaskAssignments: map[config.Task]config.TaskAssignment{config.TaskChat: {Quality: config.CostStandard}}, ActiveCloudProfile: "p", SecondaryCloudProfile: "p", CloudProfiles: []config.CloudProfile{{Name: "p", TierOverrides: map[config.CostTier]string{config.CostStandard: "chat-image"}}}}
			raw := &capture{}
			p, err := Build(c, destination, func(config.CloudProfile) (inference.Provider, error) {
				return GuardVision(raw, func(model string) bool { return model == "chat-image" }), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			target := inference.TargetForCall(p, inference.Call{Tier: "vision"})
			if target.Model != "chat-image" || !target.SupportsVision {
				t.Fatalf("image target: %+v", target)
			}
			_, err = p.Chat(context.Background(), inference.Call{Tier: "vision", Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockImage, ImageData: "fixture"}}}}})
			if err != nil || len(raw.calls) != 1 || raw.calls[0].Model != "chat-image" {
				t.Fatalf("calls=%+v err=%v", raw.calls, err)
			}
			if c.CloudProfiles[0].ImageModel != "" {
				t.Fatal("inheritance persisted an override")
			}
		})
	}
}

func TestImageInheritancePolicy(t *testing.T) {
	for _, d := range []config.Destination{config.DestinationPrimary, config.DestinationSecondary} {
		for _, tc := range []struct {
			name, override, chat, want string
			supported                  map[string]bool
		}{
			{"chat first", "", "chat", "chat", map[string]bool{"chat": true, "recommended": true}},
			{"provider recommendation", "", "text", "recommended", map[string]bool{"recommended": true}},
			{"explicit override", "explicit", "chat", "explicit", map[string]bool{"explicit": true, "chat": true}},
			{"unknown stays unavailable", "", "unknown", "", nil},
			{"explicit unknown not replaced", "unknown", "chat", "unknown", map[string]bool{"chat": true}},
		} {
			t.Run(string(d)+"/"+tc.name, func(t *testing.T) {
				// These fixtures exercise Standard chat inheritance, not the Premium product default.
				c := config.Config{TaskAssignments: map[config.Task]config.TaskAssignment{config.TaskChat: {Quality: config.CostStandard}}, ActiveCloudProfile: "p", SecondaryCloudProfile: "p", CloudProfiles: []config.CloudProfile{{Name: "p", Provider: "test", ImageModel: tc.override, TierOverrides: map[config.CostTier]string{config.CostStandard: tc.chat}}}}
				c.ModelProfiles.Cloud.Providers = map[string]config.VendorCostTiers{"test": {Standard: config.CostTierModel{Model: "recommended"}}}
				raw := &capture{}
				p, err := Build(c, d, func(config.CloudProfile) (inference.Provider, error) {
					return GuardVision(raw, func(m string) bool { return tc.supported[m] }), nil
				})
				if err != nil {
					t.Fatal(err)
				}
				target := inference.TargetForCall(p, inference.Call{Tier: "vision"})
				if target.Model != tc.want {
					t.Fatalf("target=%+v want %q", target, tc.want)
				}
				if tc.want == "" || !tc.supported[tc.want] {
					_, err = p.Chat(context.Background(), inference.Call{Tier: "vision", Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockImage, ImageData: "fixture"}}}}})
					if err == nil || len(raw.calls) != 0 {
						t.Fatalf("unconfirmed image leaked: %v %+v", err, raw.calls)
					}
				}
			})
		}
	}
}

func TestInheritedImageBackupUsesOwnEvidence(t *testing.T) {
	// These fixtures exercise Standard chat inheritance, not the Premium product default.
	c := config.Config{TaskAssignments: map[config.Task]config.TaskAssignment{config.TaskChat: {Quality: config.CostStandard}}, ActiveCloudProfile: "p", BackupCloudProfile: "b", CloudProfiles: []config.CloudProfile{
		{Name: "p", TierOverrides: map[config.CostTier]string{config.CostStandard: "primary-chat"}},
		{Name: "b", TierOverrides: map[config.CostTier]string{config.CostStandard: "backup-chat"}},
	}}
	primary, backup := &capture{fail: true}, &capture{}
	p, err := Build(c, config.DestinationPrimary, func(profile config.CloudProfile) (inference.Provider, error) {
		raw, model := primary, "primary-chat"
		if profile.Name == "b" {
			raw, model = backup, "backup-chat"
		}
		return GuardVision(raw, func(m string) bool { return m == model }), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Chat(context.Background(), inference.Call{Tier: "vision", Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockImage, ImageData: "fixture"}}}}})
	if err != nil || len(primary.calls) != 1 || len(backup.calls) != 1 || backup.calls[0].Model != "backup-chat" {
		t.Fatalf("primary=%+v backup=%+v err=%v", primary.calls, backup.calls, err)
	}
}
