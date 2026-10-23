package server

import (
	"errors"
	"strings"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DrainForUpdate owns only admission, never local installation authority. The
// caller asserts it has validated local input, acquired the OS barrier and
// independently captured initial native evidence before calling. The server
// does not verify those assertions; it verifies token shape and admission only.
// The caller must keep this stream alive even after IDLE. No token can adopt,
// release, or replace another stream's ownership (including the same token).
func (s *Server) DrainForUpdate(req *proto.DrainForUpdateRequest, stream proto.Agent_DrainForUpdateServer) error {
	token := req.GetOperationToken()
	if len(token) != 32 || strings.Trim(token, "0123456789abcdef") != "" {
		return status.Error(codes.InvalidArgument, "operation_token must be 32 lowercase hexadecimal characters")
	}
	send := func(phase proto.UpdateDrainState_Phase) error {
		return stream.Send(&proto.UpdateDrainState{OperationToken: token, Phase: phase})
	}
	release, err := s.drainForUpdateObserved(stream.Context(), func(owner uint64, phase proto.UpdateReadiness_Phase) {
		s.eventHub().setUpdateReadiness(owner, token, phase)
	}, func() error {
		if err := send(proto.UpdateDrainState_SEALED); err != nil {
			return err
		}
		return send(proto.UpdateDrainState_DRAINING)
	})
	if err != nil {
		switch {
		case errors.Is(err, errUpdateAdmissionPaused):
			return status.Error(codes.FailedPrecondition, "admission already owned or sealed")
		case errors.Is(err, errUpdateCoverageIncomplete):
			return status.Error(codes.FailedPrecondition, err.Error())
		case stream.Context().Err() != nil:
			return status.FromContextError(stream.Context().Err()).Err()
		default:
			return err
		}
	}
	defer release()
	if err := send(proto.UpdateDrainState_IDLE); err != nil {
		return err
	}
	<-stream.Context().Done()
	return status.FromContextError(stream.Context().Err()).Err()
}
