// Package invocationapproval carries a single exact-invocation authorization.
// It is deliberately context-local, unexported on the wire, and never persisted.
package invocationapproval

import (
	"bytes"
	"context"
	"sync/atomic"
)

type key struct{}
type approval struct {
	name string
	args []byte
	used atomic.Bool
}

func With(ctx context.Context, name string, args []byte) context.Context {
	return context.WithValue(ctx, key{}, &approval{name: name, args: append([]byte(nil), args...)})
}

// Consume burns the authorization even on a mismatch; it cannot authorize retries.
func Consume(ctx context.Context, name string, args []byte) bool {
	a, ok := ctx.Value(key{}).(*approval)
	return ok && !a.used.Swap(true) && a.name == name && bytes.Equal(a.args, args)
}
