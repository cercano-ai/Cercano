package server

import (
	pb "cercano/source/server/pkg/proto"
	"context"
)

// RecoverySourcePreparation observes the exact source without stopping it. Prepare
// returns a stable opaque snapshot of the recorded generation and native process
// identity (or verified absence). Preparation alone never authorizes mutation:
// recovery additionally requires ContainedSourceStop and fresh explicit consent.
type RecoverySourcePreparation interface {
	Prepare(context.Context, *pb.LifecycleIdentity) (string, error)
}
