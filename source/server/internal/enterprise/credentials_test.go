package enterprise

import (
	"errors"
	"os"
	"testing"

	"cercano/source/server/internal/secrets"
)

func TestEnterpriseCredentialGateRejectsBeforeBackend(t *testing.T) {
	for _, value := range []string{"1", "", "true", "false", "01", "2", " 0", "1\n"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv(secrets.DisableEnv, value)
			store, err := openCredentialStore(func() (CredentialStore, error) {
				t.Fatal("native enterprise backend entered")
				return nil, nil
			})
			if err != nil || store == nil {
				t.Fatalf("open: %v %v", store, err)
			}
			checkRejectedCredentialStore(t, store)
			store, err = OpenCredentialStore()
			if err != nil || store == nil {
				t.Fatalf("public open: %v", err)
			}
			checkRejectedCredentialStore(t, store)
			t.Setenv(secrets.DisableEnv, "0")
			checkRejectedCredentialStore(t, store)
		})
	}
}

func checkRejectedCredentialStore(t *testing.T, store CredentialStore) {
	t.Helper()
	if value, err := store.Get("profile"); value != "" || !errors.Is(err, secrets.ErrDisabled) {
		t.Fatalf("Get: %q %v", value, err)
	}
	if err := store.Set("profile", "synthetic"); !errors.Is(err, secrets.ErrDisabled) {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Delete("profile"); !errors.Is(err, secrets.ErrDisabled) {
		t.Fatalf("Delete: %v", err)
	}
}

func TestEnterpriseCredentialGateEnabledFakeBackend(t *testing.T) {
	for _, mode := range []string{"absent", "0"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(secrets.DisableEnv, "0")
			if mode == "absent" {
				if err := os.Unsetenv(secrets.DisableEnv); err != nil {
					t.Fatal(err)
				}
			}
			fake := secrets.NewMemory()
			calls := 0
			store, err := openCredentialStore(func() (CredentialStore, error) { calls++; return fake, nil })
			if err != nil || store != fake || calls != 1 {
				t.Fatalf("open: %v %v calls=%d", store, err, calls)
			}
			if err := store.Set("profile", "synthetic"); err != nil {
				t.Fatal(err)
			}
			if value, err := store.Get("profile"); value != "synthetic" || err != nil {
				t.Fatalf("Get: %q %v", value, err)
			}
			if err := store.Delete("profile"); err != nil {
				t.Fatal(err)
			}
			store, err = openCredentialStore(func() (CredentialStore, error) { return nil, ErrCredentialStore })
			if store != nil || err != ErrCredentialStore {
				t.Fatalf("backend error lost: %v", err)
			}
		})
	}
}
