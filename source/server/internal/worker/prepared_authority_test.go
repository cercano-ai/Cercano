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

func TestPreparedSessionContract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []string
		valid  bool
	}{
		{"missing", nil, false}, {"empty", []string{""}, false},
		{"false", []string{"false"}, false}, {"case", []string{"TRUE"}, false},
		{"duplicate", []string{"true", "true"}, false}, {"valid", []string{"true"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{preparedCredentialPolicyHeader: tc.values})
			if err := validatePreparedSession(ctx, true); (err == nil) != tc.valid {
				t.Fatalf("prepared validation=%v", err)
			}
			// Even if called without validation, the prepared process cannot select
			// the native opener when its transport metadata is stripped or corrupted.
			if workerSecretOpener(ctx, nil, true)() != nil {
				t.Fatal("prepared native fallback")
			}
		})
	}
	if err := validatePreparedSession(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	for name, server := range map[string]*WorkerServer{
		"production": NewPrepared(),
		"option":     New(WithPrepared()),
		"store":      NewWithSecretStore(secrets.NewMemory(), WithPrepared()),
		"factories":  NewWithFactories(nil, nil, WithPrepared()),
	} {
		if !server.prepared {
			t.Errorf("%s lost prepared identity", name)
		}
	}
	if New().prepared || NewWithSecretStore(secrets.NewMemory()).prepared || NewWithFactories(nil, nil).prepared {
		t.Fatal("prepared identity leaked to ordinary constructor")
	}
}

func TestPreparedWorkerCommandIdentity(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		cmd := workerCommand("/fake/cercano", "/fake/worker.sock", prepared)
		expected := []string{"/fake/cercano", "worker", "--socket", "/fake/worker.sock"}
		if prepared {
			expected = append(expected, "--prepared")
		}
		if !reflect.DeepEqual(cmd.Args, expected) || cmd.Env != nil {
			t.Fatalf("args=%v env=%v", cmd.Args, cmd.Env)
		}
	}
}
