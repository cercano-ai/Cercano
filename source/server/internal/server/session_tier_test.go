package server

import (
	"cercano/source/server/internal/credentialtest"
	"encoding/json"
	"reflect"
	"testing"

	"cercano/source/server/internal/chatroute"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

func TestSessionTierUsesEffectiveProfileAndPersists(t *testing.T) {
	credentialtest.Enable(t)
	srv, _ := newServerWithStore(t)
	cfg := config.Defaults()
	cfg.ActiveCloudProfile = "primary"
	cfg.SecondaryCloudProfile = "secondary"
	cfg.TaskAssignments = map[config.Task]config.TaskAssignment{config.TaskChat: {Destination: config.DestinationSecondary, Quality: config.CostStandard}}
	profile := func(name string) config.CloudProfile {
		return config.CloudProfile{Name: name, Flavor: "chat_completions", BaseURL: "https://example.invalid/v1", TierOverrides: map[config.CostTier]string{config.CostPremium: name + "-premium", config.CostStandard: name + "-standard", config.CostEconomy: name + "-economy"}}
	}
	cfg.CloudProfiles = []config.CloudProfile{profile("primary"), profile("secondary")}
	srv.SetConfigPersistence("", cfg)
	keys := secrets.NewMemory()
	_ = keys.Set("primary", "fixture")
	_ = keys.Set("secondary", "fixture")
	srv.SetSecrets(keys)
	call := func(action, account, model string) chatroute.Status {
		t.Helper()
		r, err := srv.SessionModel(t.Context(), &proto.SessionModelRequest{ConversationId: "tier-session", Action: action, Profile: account, Model: model})
		if err != nil {
			t.Fatal(err)
		}
		var state chatroute.Status
		if err := json.Unmarshal(r.ResultJson, &state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	s := call("status", "", "")
	if s.Effective == nil || s.Effective.Profile != "secondary" || s.Effective.Model != "secondary-standard" || len(s.Tiers) != 3 {
		t.Fatalf("wrong normal route: %+v", s)
	}
	s = call("tier", "", "premium")
	if s.Override == nil || s.Override.Model != "secondary-premium" {
		t.Fatal("tier ignored chat destination", s)
	}
	// Explicit account overrides determine subsequent tier choices.
	call("set", "primary", "custom")
	s = call("tier", "", "economy")
	if s.Override.Profile != "primary" || s.Override.Model != "primary-economy" {
		t.Fatal("lost override account", s)
	}
	if call("status", "", "").Override.Model != "primary-economy" {
		t.Fatal("override not persisted")
	}
	record := &sessionModelRunner{}
	srv.inProcessRunner = record
	for _, id := range []string{"tier-session", "other-session"} {
		if err := srv.streamProcessRequestWithToolLoop(&proto.ProcessRequestRequest{ConversationId: id, Input: "next turn"}, &fakeStream{ctx: t.Context()}); err != nil {
			t.Fatal(err)
		}
	}
	if len(record.requests) != 2 || record.requests[0].ChatRoute.Model != "primary-economy" || record.requests[1].ChatRoute != nil {
		t.Fatal("session override leaked or missing")
	}
	if !reflect.DeepEqual(srv.cfgSvc.Get().TaskAssignments, cfg.TaskAssignments) || !reflect.DeepEqual(srv.cfgSvc.Get().CloudProfiles, cfg.CloudProfiles) {
		t.Fatal("global routing mutated")
	}
	for _, bad := range []string{"", "ultra", "gpt-6-astra"} {
		if _, err := srv.SessionModel(t.Context(), &proto.SessionModelRequest{ConversationId: "tier-session", Action: "tier", Model: bad}); err == nil {
			t.Fatal("invalid tier accepted", bad)
		}
	}
	if call("clear", "", "").Override != nil {
		t.Fatal("clear failed")
	}
}

func TestSessionTierMissingModelIsNotInvented(t *testing.T) {
	srv, _ := newServerWithStore(t)
	cfg := config.Defaults()
	cfg.ActiveCloudProfile = "custom"
	cfg.CloudProfiles = []config.CloudProfile{{Name: "custom", Provider: "custom", Flavor: "chat_completions", BaseURL: "https://custom.invalid"}}
	srv.SetConfigPersistence("", cfg)
	if _, err := srv.SessionModel(t.Context(), &proto.SessionModelRequest{ConversationId: "missing", Action: "tier", Model: "premium"}); err == nil {
		t.Fatal("invented unavailable tier")
	}
}
