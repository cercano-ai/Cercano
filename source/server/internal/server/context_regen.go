package server

import (
	"context"

	"cercano/source/server/pkg/proto"
)

type regenerationStream struct {
	proto.Agent_RegenerateContextServer
	ctx  context.Context
	work *foregroundWork
}

func (s *regenerationStream) Context() context.Context { return s.ctx }
func (s *regenerationStream) Send(p *proto.RegenerateContextProgress) error {
	if p.GetDone() && p.GetOk() {
		if err := s.work.check(); err != nil {
			return err
		}
	}
	return s.Agent_RegenerateContextServer.Send(p)
}

// RegenerateContext admits ordinary regeneration before any write or producer.
// Prepared hosts retain their deny gate, including before stream access.
func (s *Server) RegenerateContext(req *proto.RegenerateContextRequest, stream proto.Agent_RegenerateContextServer) error {
	if !s.ordinaryGeneration {
		if err := s.refuseUnintegratedCoexistenceWork(); err != nil {
			return err
		}
		return s.persistSvc.RegenerateContext(req, stream)
	}
	ctx, work, err := s.beginOrdinaryServiceWork(stream.Context(), req.GetConversationId())
	if err != nil {
		return err
	}
	defer work.finish()
	return s.persistSvc.RegenerateContext(req, &regenerationStream{stream, ctx, work})
}
