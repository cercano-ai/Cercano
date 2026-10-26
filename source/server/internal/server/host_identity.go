package server

import (
	"context"

	"cercano/source/server/pkg/proto"
)

// HostIdentity identifies the agent host process serving this Server.
type HostIdentity struct {
	Profile    string
	PID        int
	InstanceID string
	Version    string
}

// SetHostIdentity records the identity GetHostIdentity reports. Call it before
// the server starts serving.
func (s *Server) SetHostIdentity(id HostIdentity) {
	s.hostIdentity.Store(&id)
}

// GetHostIdentity reports the profile, process and instance this host serves.
// It reads one atomically stored value and takes no lock, so it answers at
// once whatever the host is doing.
func (s *Server) GetHostIdentity(ctx context.Context, _ *proto.GetHostIdentityRequest) (*proto.HostIdentity, error) {
	id := s.hostIdentity.Load()
	if id == nil {
		return &proto.HostIdentity{}, nil
	}
	return &proto.HostIdentity{
		Profile:    id.Profile,
		Pid:        int32(id.PID),
		InstanceId: id.InstanceID,
		Version:    id.Version,
	}, nil
}
