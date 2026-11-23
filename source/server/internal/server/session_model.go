package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cercano/source/server/internal/chatroute"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

func (s *Server) sessionModel(ctx context.Context, convID string, req chatroute.Request) (chatroute.Status, error) {
	store, ok := s.persistSvc.Store().(chatroute.Store)
	if !ok {
		return chatroute.Status{}, fmt.Errorf("session model persistence unavailable")
	}
	if convID == "" {
		return chatroute.Status{}, fmt.Errorf("conversation id required")
	}
	if req.Action == "" {
		req.Action = "status"
	}
	// Tier choices are resolved on the server, never guessed by the client.
	if req.Action == "tier" {
		quality := config.CostTier(req.Model)
		if quality != config.CostPremium && quality != config.CostStandard && quality != config.CostEconomy {
			return chatroute.Status{}, fmt.Errorf("unknown chat quality %q", req.Model)
		}
		current, err := store.ChatRoute(ctx, convID)
		if err != nil {
			return chatroute.Status{}, err
		}
		route := sessionTierRoute(s.cfgSvc.Get(), current)
		if req.Profile != "" {
			route.Profile = req.Profile
		}
		profile, ok := s.cfgSvc.Get().Profile(route.Profile)
		if !ok {
			return chatroute.Status{}, fmt.Errorf("no saved cloud profile for session tier")
		}
		model := s.cfgSvc.Get().ModelProfiles.ResolveCloudModelForTier(profile, quality.CapabilityTier())
		if model == "" {
			return chatroute.Status{}, fmt.Errorf("%s tier has no configured model for %s", quality, profile.Name)
		}
		req = chatroute.Request{Action: "set", Profile: profile.Name, Model: model}
	}
	switch req.Action {
	case "models":
		if strings.TrimSpace(req.Profile) == "" || req.Model != "" {
			return chatroute.Status{}, fmt.Errorf("models requires a saved profile name and no model")
		}
		catalog, err := s.ListCloudProfileModels(ctx, &proto.ListCloudProfileModelsRequest{ProfileName: req.Profile})
		if err != nil {
			return chatroute.Status{}, err
		}
		if catalog.GetError() != "" {
			return chatroute.Status{}, fmt.Errorf("model catalog: %s", catalog.GetError())
		}
		result, err := s.sessionModel(ctx, convID, chatroute.Request{Action: "status"})
		if err != nil {
			return chatroute.Status{}, err
		}
		result.Note = "Available model IDs for " + req.Profile + ". No routing changes made."
		for _, m := range catalog.Models {
			result.Models = append(result.Models, chatroute.Model{ID: m.GetId(), Name: m.GetDisplayName()})
		}
		return result, nil
	case "set":
		route := chatroute.Route{Profile: strings.TrimSpace(req.Profile), Model: strings.TrimSpace(req.Model)}
		resolver, ok := s.providerSvc.(chatroute.Resolver)
		if !ok {
			return chatroute.Status{}, fmt.Errorf("session model routing unavailable")
		}
		if err := s.validateMistralSelection(ctx, route.Profile, route.Model); err != nil {
			return chatroute.Status{}, err
		}
		if _, err := resolver.ResolveChatRoute(ctx, route); err != nil {
			return chatroute.Status{}, err
		}
		if err := store.SetChatRoute(ctx, convID, &route); err != nil {
			if conversation.IsMutationFailure(err) {
				persistsvc.ReportWriteFailure(ctx, err)
			}
			return chatroute.Status{}, err
		}
	case "clear", "status":
		if req.Profile != "" || req.Model != "" {
			return chatroute.Status{}, fmt.Errorf("profile/model are only valid for set")
		}
		if req.Action == "clear" {
			if err := store.SetChatRoute(ctx, convID, nil); err != nil {
				if conversation.IsMutationFailure(err) {
					persistsvc.ReportWriteFailure(ctx, err)
				}
				return chatroute.Status{}, err
			}
		}
	default:
		return chatroute.Status{}, fmt.Errorf("unknown session model action %q (want status, models, set, tier, clear)", req.Action)
	}
	route, err := store.ChatRoute(ctx, convID)
	if err != nil {
		return chatroute.Status{}, err
	}
	result := chatroute.Status{Override: route, Note: "Normal main-chat routing. Changes apply to the next message; delegation and background routing are unchanged."}
	if route != nil {
		result.Note = "Session main-chat override; applies to the next message and persists on resume. No fallback. Delegation and background routing are unchanged. Model availability is checked by the provider on the next request."
	}
	cfg := s.cfgSvc.Get()
	effective := sessionTierRoute(cfg, route)
	if p, ok := cfg.Profile(effective.Profile); ok {
		result.Effective = &effective
		for _, q := range []config.CostTier{config.CostPremium, config.CostStandard, config.CostEconomy} {
			result.Tiers = append(result.Tiers, chatroute.TierChoice{Tier: string(q), Model: cfg.ModelProfiles.ResolveCloudModelForTier(p, q.CapabilityTier())})
		}
	}
	for _, p := range cfg.CloudProfiles {
		result.Profiles = append(result.Profiles, chatroute.Profile{Name: p.Name, Provider: p.Provider, Model: cfg.ModelProfiles.ResolveCloudModelForTier(p, cfg.TaskAssignment(config.TaskChat).Quality.CapabilityTier())})
	}
	return result, nil
}

// SessionModel is a direct user control, usable even when the selected model is
// unavailable. Agent-initiated changes use the permission-gated capability.
func (s *Server) SessionModel(ctx context.Context, req *proto.SessionModelRequest) (*proto.SessionModelResponse, error) {
	if s.persistSvc.Store() == nil {
		return nil, fmt.Errorf("session model persistence unavailable")
	}
	// CLI sessions may not have sent their first message yet.
	if req.GetConversationId() == "" {
		return nil, fmt.Errorf("conversation id required")
	}
	if req.GetAction() == "set" || req.GetAction() == "tier" {
		if err := s.persistSvc.Store().EnsureConversation(ctx, req.GetConversationId(), req.GetWorkDir(), ""); err != nil {
			return nil, err
		}
	}
	result, err := s.sessionModel(ctx, req.GetConversationId(), chatroute.Request{Action: req.GetAction(), Profile: req.GetProfile(), Model: req.GetModel()})
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &proto.SessionModelResponse{ResultJson: data}, nil
}

func sessionTierRoute(cfg config.Config, override *chatroute.Route) chatroute.Route {
	if override != nil {
		return *override
	}
	assignment := cfg.TaskAssignment(config.TaskChat)
	name, _ := cfg.DestinationProfiles(assignment.Destination)
	if name == "" {
		name = cfg.ActiveCloudProfile
	}
	p, ok := cfg.Profile(name)
	if !ok {
		return chatroute.Route{}
	}
	return chatroute.Route{Profile: name, Model: cfg.ModelProfiles.ResolveCloudModelForTier(p, assignment.Quality.CapabilityTier())}
}
