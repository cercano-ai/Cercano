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
// SetActiveCloudProfile's shape: validate, mutate, rebuild, persist.
func (s *Server) SetBackupCloudProfile(ctx context.Context, req *proto.SetBackupCloudProfileRequest) (*proto.SetBackupCloudProfileResponse, error) {
	if err := s.refuseUnintegratedCoexistenceWork(); err != nil {
		return nil, err
	}
	name := req.GetName()
	if name != "" {
		exists, isActive := s.cfgSvc.ProfileInfo(name)
		if !exists {
			return &proto.SetBackupCloudProfileResponse{Ok: false, Error: fmt.Sprintf("no profile %q", name)}, nil
		}
		if isActive {
			return &proto.SetBackupCloudProfileResponse{Ok: false, Error: "backup profile cannot be the active profile"}, nil
		}
	}
	if !s.cfgSvc.SetBackupProfile(name) {
		return &proto.SetBackupCloudProfileResponse{Ok: false, Error: "backup profile update rejected (configuration frozen or profile unavailable)"}, nil
	}
	if err := s.rebuildCloud(); err != nil {
		// The backup name is set but the provider chain couldn't be rebuilt —
		// report it, keep the config (mirrors SetActiveCloudProfile).
		if persistErr := s.cfgSvc.Persist(); persistErr != nil {
			return &proto.SetBackupCloudProfileResponse{Ok: false, Error: persistErr.Error()}, nil
		}
		return &proto.SetBackupCloudProfileResponse{Ok: false, Error: err.Error()}, nil
	}
	if persistErr := s.cfgSvc.Persist(); persistErr != nil {
		return &proto.SetBackupCloudProfileResponse{Ok: false, Error: persistErr.Error()}, nil
	}
	return &proto.SetBackupCloudProfileResponse{Ok: true}, nil
}
