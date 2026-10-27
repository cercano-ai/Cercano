package worker

import (
	toolssvc "cercano/source/server/internal/hostsvc/tools"
	"context"
	"fmt"

	"cercano/source/server/internal/capabilities"
	"google.golang.org/grpc/metadata"
)

const preparedCredentialPolicyHeader = "cercano-worker-host-credentials-only"

// ValidatePrepared rejects legacy discovery and verifies the pinned generation
// before installing a prepared runner. Spawn verifies it again on every launch.
func (p SpawnPolicy) ValidatePrepared() error {
	if !p.pinned {
		return fmt.Errorf("prepared worker requires a pinned spawn policy")
	}
	_, err := p.workerBinary()
	return err
}

// TurnAuthority is instantiated per RunTurn. It must never store authority in a
// runner-global context. Context is consulted before descendant writes.
type TurnAuthority interface {
	Context(context.Context, string) (context.Context, error)
	Close(error)
}
type TurnAuthoritySetter interface {
	SetTurnAuthority(func(context.Context, string) (TurnAuthority, error))
}

func (w *workerRunner) SetTurnAuthority(f func(context.Context, string) (TurnAuthority, error)) {
	w.turnAuthority = f
}

// The flag travels on the worker session, not in ambient process configuration.
// It only removes direct capability keychain access; provider credential requests
// continue to use the existing host credential proxy. Explicit injection wins.
func workerSecretOpener(ctx context.Context, injected func() capabilities.SecretStore) func() capabilities.SecretStore {
	if injected != nil {
		return injected
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get(preparedCredentialPolicyHeader)) != 0 {
		return func() capabilities.SecretStore { return nil }
	}
	return openWorkerSecrets
}

// Prepared worker-local dispatch is nested, not the host independent task path.
// Reject before the legacy asynchronous ensure proxy can start provider work.
func configurePreparedDispatch(ctx context.Context, tools any) {
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get(preparedCredentialPolicyHeader)) == 0 {
		return
	}
	if scoped, ok := tools.(interface{ SetDispatchScope(toolssvc.DispatchScope) }); ok {
		scoped.SetDispatchScope(func(context.Context, string, string) (context.Context, func(error) error, error) {
			return nil, nil, fmt.Errorf("prepared worker nested dispatch is not supported")
		})
	}
}
