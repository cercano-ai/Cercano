package agenttools

import "context"

type delegationKey struct{}

// DelegationScope is host-owned authority, never model-supplied arguments.
// Registry is the immediate parent's catalog, including lookup-only aliases.
type DelegationScope struct {
	Registry *Registry
	ReadOnly bool
	Accepted func(string)
}

func WithDelegationScope(ctx context.Context, scope DelegationScope) context.Context {
	return context.WithValue(ctx, delegationKey{}, scope)
}
func DelegationFrom(ctx context.Context) (DelegationScope, bool) {
	scope, ok := ctx.Value(delegationKey{}).(DelegationScope)
	return scope, ok
}
