package server

import (
	"context"
	"fmt"

	"cercano/source/server/pkg/proto"
)

// The resilience engine itself is built by the providers service
// (internal/hostsvc/providers.wrapResilience) during Rebuild — this file only
// carries the RPC that names the backup profile. A near-identical
// wrapBackupLocked used to live here with no callers; it was deleted so the
// live builder can't silently drift from a dead twin.

// SetBackupCloudProfile implements proto.AgentServer — names the fallback
// profile (empty clears it) and rewires the provider chain. Mirrors
// SetActiveCloudProfile's shape: validate, publish, apply, rebuild.
func (s *Server) SetBackupCloudProfile(ctx context.Context, req *proto.SetBackupCloudProfileRequest) (*proto.SetBackupCloudProfileResponse, error) {
	s.configApplyMu.Lock()
	defer s.configApplyMu.Unlock()
	ctx, work, err := s.beginOrdinaryServiceWork(ctx, "")
	if err != nil {
		return nil, err
	}
	defer work.finish()
	name := req.GetName()
	candidate := s.cfgSvc.Get()
	if name != "" {
		if _, exists := profileByName(candidate.CloudProfiles, name); !exists {
			return &proto.SetBackupCloudProfileResponse{Ok: false, Error: fmt.Sprintf("no profile %q", name)}, nil
		}
		if candidate.ActiveCloudProfile == name {
			return &proto.SetBackupCloudProfileResponse{Ok: false, Error: "backup profile cannot be the active profile"}, nil
		}
	}
	if name == "" {
		candidate.SetPrimaryBackups(nil)
	} else {
		candidate.SetPrimaryBackups([]string{name})
	}
	if err := s.cfgSvc.SaveAndSet(candidate); err != nil {
		return &proto.SetBackupCloudProfileResponse{Ok: false, Error: err.Error()}, nil
	}
	if err := s.rebuildCloud(); err != nil {
		return &proto.SetBackupCloudProfileResponse{Ok: false, Error: fmt.Sprintf("configuration saved but provider rebuild failed: %v", err)}, nil
	}
	return &proto.SetBackupCloudProfileResponse{Ok: true}, nil
}
