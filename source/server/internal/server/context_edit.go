package server

import (
	"context"

	"cercano/source/server/pkg/proto"
)

// ProposeContextEdit implements proto.AgentServer — delegates to persistSvc.
func (s *Server) ProposeContextEdit(ctx context.Context, req *proto.ProposeContextEditRequest) (*proto.ProposeContextEditResponse, error) {
	return s.persistSvc.ProposeContextEdit(ctx, req)
}

// DeleteConversationTurns implements proto.AgentServer — delegates to persistSvc.
func (s *Server) DeleteConversationTurns(ctx context.Context, req *proto.DeleteConversationTurnsRequest) (*proto.DeleteConversationTurnsResponse, error) {
	ctx, work, err := s.beginOrdinaryServiceWork(ctx, req.GetConversationId())
	if err != nil {
		return nil, err
	}
	defer work.finish()
	return s.persistSvc.DeleteConversationTurns(ctx, req)
}
