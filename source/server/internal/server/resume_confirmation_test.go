package server

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// No production processes are inspected or signalled by this adapter.
type resumeProcessFixture struct {
	identity     string
	stops        int
	dead         bool
	onStop       func()
	lostResponse bool
	remainAlive  bool
}

func (p *resumeProcessFixture) Prepare(context.Context, *pb.LifecycleIdentity) (string, error) {
	return p.identity, nil
}
func (p *resumeProcessFixture) Stop(ctx context.Context, source *pb.LifecycleIdentity, expected string, validate func() error) error {
	if expected != p.identity {
		return status.Error(codes.FailedPrecondition, "PID/start identity changed")
	}
	if err := validate(); err != nil {
		return err
	}
	p.stops++
	p.dead = !p.remainAlive
	if p.onStop != nil {
		p.onStop()
	}
	if p.lostResponse {
		return status.Error(codes.Unavailable, "stop response lost")
	}
	return nil
}
