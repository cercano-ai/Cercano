package worker

import (
	"cercano/source/server/internal/capabilities"
	toolssvc "cercano/source/server/internal/hostsvc/tools"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"testing"
)

// Run this probe with credentials disabled before enabling integration fixtures.
// The sentinel proves full tool assembly reaches only the supplied opener.
func TestWorkerSecretOpenerTrap(t *testing.T) {
	const sentinel = "injected opener reached"
	w := NewWithSecretStore(secrets.NewMemory())
	if w.secretOpener == nil || w.secretOpener() == nil {
		t.Fatal("missing injected store")
	}
	defer func() {
		if got := recover(); got != sentinel {
			t.Fatalf("opener trap = %v", got)
		}
	}()
	buildWorkerToolSvcWithSecrets(nil, nil, nil, nil, nil, config.Defaults(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, func() capabilities.SecretStore { panic(sentinel) })
	t.Fatal("opener was not called")
}

// A failed injected opener is authoritative: assembly must not retry natively.
func TestWorkerSecretOpenerNilIsAuthoritative(t *testing.T) {
	calls := 0
	svc := buildWorkerToolSvcWithSecrets(nil, nil, nil, nil, nil, config.Defaults(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, func() capabilities.SecretStore { calls++; return nil })
	if calls != 1 || svc.(toolssvc.Catalog).CapRegistry().Services().Secrets != nil {
		t.Fatal("injected credential result was not preserved")
	}
}
