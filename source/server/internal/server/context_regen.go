package server

import (
	"cercano/source/server/pkg/proto"
)

// RegenerateContext implements proto.AgentServer — delegates to persistSvc.
func (s *Server) RegenerateContext(req *proto.RegenerateContextRequest, stream proto.Agent_RegenerateContextServer) error {
	// The generator can reschedule ID-only timers, losing authority and its blocker.
	// Refuse before even scheduling; preserving just the initial context is insufficient.
	if err := s.refuseUnintegratedCoexistenceWork(); err != nil {
		return err
	}
	return s.persistSvc.RegenerateContext(req, stream)
}
