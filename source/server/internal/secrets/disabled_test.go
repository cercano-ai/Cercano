package secrets

import (
	"errors"
	"os"
	"testing"

	"github.com/99designs/keyring"
)

func TestCredentialGateRejectsBeforeBackend(t *testing.T) {
	for _, value := range []string{"1", "", "true", "false", "01", "2", " 0", "1\n"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv(DisableEnv, value)
			validation := ValidateEnvironment()
			if value == "1" && validation != nil {
				t.Fatal(validation)
			}
			if value != "1" && !errors.Is(validation, ErrDisabled) {
				t.Fatalf("validation: %v", validation)
			}
			if !errors.Is(CheckAllowed(), ErrDisabled) {
				t.Fatal("gate allowed credentials")
			}
			store, err := openKeychain(func(keyring.Config) (keyring.Keyring, error) {
				t.Fatal("native backend entered")
				return nil, nil
			})
			if err != nil || store == nil {
				t.Fatalf("open: %v, %v", store, err)
			}
			checkRejectedStore(t, store)
			// The public path is safe only with a rejecting environment.
			store, err = OpenKeychain()
			if err != nil || store == nil {
				t.Fatalf("public open: %v", err)
			}
			checkRejectedStore(t, store)
			t.Setenv(DisableEnv, "0")
			checkRejectedStore(t, store) // A sentinel cannot become enabled later.
		})
	}
}

func checkRejectedStore(t *testing.T, store Store) {
	t.Helper()
	if value, err := store.Get("profile"); value != "" || !errors.Is(err, ErrDisabled) {
		t.Fatalf("Get: %q, %v", value, err)
	}
	if err := store.Set("profile", "synthetic"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Delete("profile"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Delete: %v", err)
	}
	if names, err := store.List(); names != nil || !errors.Is(err, ErrDisabled) {
		t.Fatalf("List: %v, %v", names, err)
	}
}

func TestCredentialGateEnabledFakeBackend(t *testing.T) {
	for _, mode := range []string{"absent", "0"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(DisableEnv, "0")
			if mode == "absent" {
				if err := os.Unsetenv(DisableEnv); err != nil {
					t.Fatal(err)
				}
			}
			if err := ValidateEnvironment(); err != nil {
				t.Fatal(err)
			}
			if err := CheckAllowed(); err != nil {
				t.Fatal(err)
			}
			calls := 0
			store, err := openKeychain(func(config keyring.Config) (keyring.Keyring, error) {
				calls++
				if config.ServiceName != service || !config.KeychainTrustApplication {
					t.Fatalf("config: %+v", config)
				}
				return keyring.NewArrayKeyring(nil), nil
			})
			if err != nil || calls != 1 {
				t.Fatalf("open: calls=%d err=%v", calls, err)
			}
			if err := store.Set("profile", "synthetic"); err != nil {
				t.Fatal(err)
			}
			if value, err := store.Get("profile"); value != "synthetic" || err != nil {
				t.Fatalf("Get: %q %v", value, err)
			}
			if names, err := store.List(); len(names) != 1 || err != nil {
				t.Fatalf("List: %v %v", names, err)
			}
			if err := store.Delete("profile"); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("fake backend failure")
			store, err = openKeychain(func(keyring.Config) (keyring.Keyring, error) { return nil, failure })
			if store != nil || !errors.Is(err, failure) {
				t.Fatalf("backend error lost: %v", err)
			}
		})
	}
}
