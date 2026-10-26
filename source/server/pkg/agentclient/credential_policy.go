package agentclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"cercano/source/server/internal/secrets"
)

// ErrCredentialIsolation means the connection must not be used or replaced by
// an auto-launch fallback. It is not evidence that the agent is absent.
var ErrCredentialIsolation = errors.New("agent credential isolation required")

func validateCredentialEnvironment() error {
	if err := secrets.ValidateEnvironment(); err != nil {
		return fmt.Errorf("%w: %v", ErrCredentialIsolation, err)
	}
	return nil
}

func credentialIsolationRequired() bool { return secrets.CheckAllowed() != nil }

// A positive attestation applies only to this transport. gRPC must not silently
// replace it with a different host on the same address; application reconnect
// creates a fresh ClientConn and attests again. Failed TCP dials may retry.
func singleTransportDialer(dial func(context.Context, string) (net.Conn, error)) func(context.Context, string) (net.Conn, error) {
	var mu sync.Mutex
	used := false
	return func(ctx context.Context, addr string) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		if used {
			return nil, fmt.Errorf("%w: transport replacement requires fresh attestation", ErrCredentialIsolation)
		}
		conn, err := dial(ctx, addr)
		if err == nil {
			used = true
		}
		return conn, err
	}
}
