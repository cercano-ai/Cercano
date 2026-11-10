package server

import (
	pb "cercano/source/server/pkg/proto"
	"context"
)

// LegacyProcessStop is a trusted, injected platform boundary, not signal authority
// derived from argv alone. Prepare must prove UID, PID/start identity, executable
// and exact host attribution and return a stable opaque snapshot. Stop must hold
// the source launch exclusion, revalidate that complete snapshot, call validate
// immediately before any signal, verify death, and fail closed on ambiguity or
// unsupported platforms. RecoveryGuard independently proves absence afterward.
// Production reuses the launcher single-target verified shutdown machinery.
type LegacyProcessStop interface {
	Prepare(context.Context, *pb.LifecycleIdentity) (string, error)
	Stop(context.Context, *pb.LifecycleIdentity, string, func() error) error
}
