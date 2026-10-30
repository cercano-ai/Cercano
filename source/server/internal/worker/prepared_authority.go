package worker

import (
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	toolssvc "cercano/source/server/internal/hostsvc/tools"
	"context"
	"errors"
	"fmt"
	"sync"

	"cercano/source/server/internal/capabilities"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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

// DescendantAuthority acquires exact host-side scopes; stream IDs alone never
// authorize creation. Implementations retain completion until Close.
type DescendantAuthority interface {
	Ensure(context.Context, string, string, string, string, []string) error
}

type TurnAuthoritySetter interface {
	SetTurnAuthority(func(context.Context, string) (TurnAuthority, error))
}

func (w *workerRunner) SetTurnAuthority(f func(context.Context, string) (TurnAuthority, error)) {
	w.turnAuthority = f
}

// validatePreparedSession never infers prepared process identity from metadata.
// Ordinary workers still accept legacy sessions without this header.
func validatePreparedSession(ctx context.Context, prepared bool) error {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(preparedCredentialPolicyHeader)
	if !prepared && len(values) == 0 {
		return nil
	}
	if len(values) != 1 || values[0] != "true" {
		return status.Error(codes.FailedPrecondition, "worker: prepared credential policy must be exactly true")
	}
	return nil
}

// Prepared identity is constructor-pinned; the session header is validated
// separately. Both suppress direct capability keychain access; provider requests
// continue to use the existing host credential proxy. Explicit injection wins.
func workerSecretOpener(ctx context.Context, injected func() capabilities.SecretStore, prepared ...bool) func() capabilities.SecretStore {
	if injected != nil {
		return injected
	}
	md, _ := metadata.FromIncomingContext(ctx)
	if (len(prepared) > 0 && prepared[0]) || len(md.Get(preparedCredentialPolicyHeader)) != 0 {
		return func() capabilities.SecretStore { return nil }
	}
	return openWorkerSecrets
}

// Worker-local scopes are producer joins, never ownership grants. The host alone
// acquires capabilities when it drains Ensure; the worker fences and joins every
// dispatch callback before flushing the sender and publishing its terminal result.
type preparedDispatchLifetimeKey struct{}
type preparedDispatchProducerKey struct{}
type preparedDispatchProducer struct {
	lifetime *preparedDispatchLifetime
	closed   bool
}
type preparedDispatchLifetime struct {
	mu     sync.Mutex
	closed bool
	active sync.WaitGroup
	err    error
}

func (d *preparedDispatchLifetime) begin(ctx context.Context, _, _ string) (context.Context, func(error) error, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	producer, _ := ctx.Value(preparedDispatchProducerKey{}).(*preparedDispatchProducer)
	if (producer != nil && (producer.lifetime != d || producer.closed)) || (d.closed && producer == nil) {
		return nil, nil, fmt.Errorf("prepared worker dispatch lifetime closed")
	}
	d.active.Add(1)
	producer = &preparedDispatchProducer{lifetime: d}
	ctx = context.WithValue(persistsvc.WithWriteFailure(ctx, nil), preparedDispatchProducerKey{}, producer)
	var once sync.Once
	var result error
	return ctx, func(err error) error {
		once.Do(func() {
			result = errors.Join(err, persistsvc.WriteFailure(ctx))
			d.mu.Lock()
			producer.closed = true
			d.err = errors.Join(d.err, result)
			d.mu.Unlock()
			d.active.Done()
		})
		return result
	}, nil
}

func (d *preparedDispatchLifetime) join() error {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
	d.active.Wait()
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.err
}

func configurePreparedDispatch(ctx context.Context, tools any, prepared ...bool) {
	md, _ := metadata.FromIncomingContext(ctx)
	if !(len(prepared) > 0 && prepared[0]) && len(md.Get(preparedCredentialPolicyHeader)) == 0 {
		return
	}
	if scoped, ok := tools.(interface{ SetDispatchScope(toolssvc.DispatchScope) }); ok {
		lifetime, _ := ctx.Value(preparedDispatchLifetimeKey{}).(*preparedDispatchLifetime)
		if lifetime == nil {
			scoped.SetDispatchScope(func(context.Context, string, string) (context.Context, func(error) error, error) {
				return nil, nil, fmt.Errorf("prepared worker dispatch lifetime unavailable")
			})
			return
		}
		scoped.SetDispatchScope(lifetime.begin)
	}
}
