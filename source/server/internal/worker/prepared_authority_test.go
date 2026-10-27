package worker

import (
	"context"
	"reflect"
	"testing"

	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/secrets"
	"google.golang.org/grpc/metadata"
)

func TestPreparedWorkerCredentialPolicy(t *testing.T) {
	// Never invoke the ordinary opener in this test: compare its identity only.
	ordinary := workerSecretOpener(context.Background(), nil)
	if reflect.ValueOf(ordinary).Pointer() != reflect.ValueOf(openWorkerSecrets).Pointer() {
		t.Fatal("ordinary default changed")
	}
	prepared := metadata.NewIncomingContext(context.Background(), metadata.Pairs(preparedCredentialPolicyHeader, "true"))
	if workerSecretOpener(prepared, nil)() != nil {
		t.Fatal("prepared session exposed native credentials")
	}
	store := secrets.NewMemory()
	calls := 0
	injected := func() capabilities.SecretStore { calls++; return store }
	if workerSecretOpener(prepared, injected)() != store || calls != 1 {
		t.Fatal("prepared policy broke explicit test injection")
	}
	if reflect.ValueOf(workerSecretOpener(context.Background(), nil)).Pointer() != reflect.ValueOf(ordinary).Pointer() {
		t.Fatal("session policy leaked globally")
	}
}

func TestPreparedWorkerRejectsLegacyPolicy(t *testing.T) {
	if err := (SpawnPolicy{}).ValidatePrepared(); err == nil {
		t.Fatal("legacy discovery accepted")
	}
	policy, err := NewSpawnPolicy(policyFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.ValidatePrepared(); err != nil {
		t.Fatal(err)
	}
}
