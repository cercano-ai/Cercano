package server

import (
	"cercano/source/server/internal/routingwire"
	"cercano/source/server/pkg/proto"
	"context"
)

// UpdateRoutingAssignments atomically saves a complete routing draft. Absence
// preserves existing assignments; a present empty draft explicitly clears them.
func (s *Server) UpdateRoutingAssignments(ctx context.Context, req *proto.UpdateRoutingAssignmentsRequest) (*proto.UpdateRoutingAssignmentsResponse, error) {
	s.configApplyMu.Lock()
	defer s.configApplyMu.Unlock()
	if err := s.refuseUnintegratedCoexistenceWork(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.GetAssignments() == nil {
		return &proto.UpdateRoutingAssignmentsResponse{Ok: true}, nil
	}
	candidate := s.cfgSvc.Get()
	routingwire.ApplyAssignments(&candidate, req.Assignments)
	if err := candidate.ValidateRouting(); err != nil {
		return &proto.UpdateRoutingAssignmentsResponse{Error: err.Error()}, nil
	}
	if err := s.cfgSvc.SaveAndSet(candidate); err != nil {
		return &proto.UpdateRoutingAssignmentsResponse{Error: err.Error()}, nil
	}
	response := &proto.UpdateRoutingAssignmentsResponse{Ok: true}
	if err := s.rebuildCloud(); err != nil {
		response.Warning = err.Error()
	}
	s.broadcastConfigChanged("routing_assignments", "")
	return response, nil
}
