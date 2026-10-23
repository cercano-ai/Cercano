package agentclient

import (
	"context"
	"strings"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UpdateDrain owns one reversible seal. Close MUST be called, including after
// IDLE. Recv has one reader; Close may be concurrent. No goroutines, retry,
// reconnect, auto-launch, commit, or process actions are performed here.
type UpdateDrain struct {
	stream proto.Agent_DrainForUpdateClient
	cancel context.CancelFunc
	token  string
	next   proto.UpdateDrainState_Phase
}

// OpenUpdateDrain uses an already connected Agent client. The caller supplies
// an operation identity (32 lowercase hex characters), not an authorization.
// Context cancellation/deadline also closes ownership. Opening alone is not
// acknowledgement: receive SEALED, DRAINING, then IDLE, checking every error.
func OpenUpdateDrain(ctx context.Context, agent proto.AgentClient, token string) (*UpdateDrain, error) {
	if len(token) != 32 || strings.Trim(token, "0123456789abcdef") != "" {
		return nil, status.Error(codes.InvalidArgument, "operation_token must be 32 lowercase hexadecimal characters")
	}
	ctx, cancel := context.WithCancel(ctx)
	stream, err := agent.DrainForUpdate(ctx, &proto.DrainForUpdateRequest{OperationToken: token})
	if err != nil {
		cancel()
		return nil, err
	}
	return &UpdateDrain{stream: stream, cancel: cancel, token: token, next: proto.UpdateDrainState_SEALED}, nil
}

func (d *UpdateDrain) Close() error { d.cancel(); return nil }

func (d *UpdateDrain) Recv() (*proto.UpdateDrainState, error) {
	frame, err := d.stream.Recv()
	if err != nil {
		d.cancel()
		return nil, err
	}
	if frame.GetOperationToken() != d.token || frame.GetPhase() != d.next || d.next > proto.UpdateDrainState_IDLE {
		d.cancel()
		return nil, status.Error(codes.DataLoss, "unexpected update drain token or phase")
	}
	d.next++
	return frame, nil
}
