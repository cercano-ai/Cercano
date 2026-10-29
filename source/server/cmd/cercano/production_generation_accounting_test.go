package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/telemetry"
	"cercano/source/server/internal/usage"
)

func TestProductionGenerationAccountingDrain(t *testing.T) {
	o, _ := developmentProductionFixture(t)
	policy := canonicalGenerationStartup()
	policy.authorities = productionAuthorityResolvers{
		configPath:       func() string { return filepath.Join(o.State, "config.yaml") },
		conversationPath: func() (string, error) { return filepath.Join(o.State, "conversations.db"), nil },
	}
	admission, err := policy.admit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Accounting admission must not enable canonical application activation.
	if admission.validateActivation() == nil {
		t.Fatal("parity gate removed")
	}
	path := filepath.Join(o.State, "telemetry.db")
	d := productionAccountingDependencies{open: func(got string) (*telemetry.SQLiteStore, error) {
		if got != path {
			t.Fatalf("accounting authority = %s", got)
		}
		return telemetry.NewSQLiteStore(got)
	}}
	a, closeA, err := composeProductionAccounting(admission, "A", d)
	if err != nil {
		t.Fatal(err)
	}
	defer closeA()
	b, closeB, err := composeProductionAccounting(admission, "B", d)
	if err != nil {
		t.Fatal(err)
	}
	defer closeB()
	observation := usage.AttemptObservation{ID: "same-attempt", Revision: 1, StartedAt: time.Now(), Outcome: usage.Started}
	if !a.EmitAttempt(observation) {
		t.Fatal("A rejected observation")
	}
	if err := closeA(); err != nil {
		t.Fatal(err)
	}
	// A's drain does not close B; duplicate delivery remains revision-idempotent.
	if !b.EmitAttempt(observation) {
		t.Fatal("B rejected retry")
	}
	observation.ID = "B-after-A"
	if !b.EmitAttempt(observation) {
		t.Fatal("B rejected new work")
	}
	if err := closeB(); err != nil {
		t.Fatal(err)
	}
	reopened, err := telemetry.NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for _, id := range []string{"same-attempt", "B-after-A"} {
		if _, err := reopened.AccountingAttempt(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProductionGenerationAccountingHostLifecycle(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "retirement", true: "open-failure"}[fail], func(t *testing.T) {
			o, auth := developmentProductionFixture(t)
			var store *telemetry.SQLiteStore
			calls := 0
			injected := errors.New("injected accounting open failure")
			d := enabledDevelopmentFakes(t, developmentProductionDependencies{accounting: productionAccountingDependencies{open: func(path string) (*telemetry.SQLiteStore, error) {
				calls++
				if path != filepath.Join(o.State, "telemetry.db") {
					t.Fatalf("wrong authority: %s", path)
				}
				if fail {
					return nil, injected
				}
				var err error
				store, err = telemetry.NewSQLiteStore(":memory:")
				return store, err
			}}})
			srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, d)
			if cleanup != nil {
				defer cleanup.Wait(context.Background())
			}
			if calls != 1 {
				t.Fatalf("accounting opens = %d", calls)
			}
			if fail {
				if !errors.Is(err, injected) || srv != nil {
					t.Fatalf("failure not propagated: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.GetStats(context.Background()); err != nil {
				t.Fatalf("collector closed early: %v", err)
			}
			if err := srv.RetireCoexistence(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err := store.GetStats(context.Background()); err == nil {
				t.Fatal("retirement did not close accounting store")
			}
			if err := cleanup.Wait(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
