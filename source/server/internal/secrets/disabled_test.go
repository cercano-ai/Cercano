package secrets

import (
	"errors"
	"github.com/99designs/keyring"
	"os"
	"testing"
)

type backendSpy struct {
	keyring.Keyring
	calls int
}

func (s *backendSpy) Get(string) (keyring.Item, error) {
	s.calls++
	return keyring.Item{Data: []byte("fake")}, nil
}
func (s *backendSpy) Set(keyring.Item) error  { s.calls++; return nil }
func (s *backendSpy) Remove(string) error     { s.calls++; return nil }
func (s *backendSpy) Keys() ([]string, error) { s.calls++; return []string{"fake"}, nil }

func TestCredentialBoundary(t *testing.T) {
	for _, value := range []string{"1", "", "true", " 1", "2", "0", "absent"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(DisableEnv, value)
			if value == "absent" {
				os.Unsetenv(DisableEnv)
			}
			spy := &backendSpy{}
			opens := 0
			store, err := openKeychain(func(keyring.Config) (keyring.Keyring, error) { opens++; return spy, nil })
			if err != nil {
				t.Fatal(err)
			}
			_, getErr := store.Get("profile")
			setErr := store.Set("profile", "secret")
			deleteErr := store.Delete("profile")
			_, listErr := store.List()
			disabled := value != "0" && value != "absent"
			for _, e := range []error{getErr, setErr, deleteErr, listErr} {
				if errors.Is(e, ErrDisabled) != disabled {
					t.Fatalf("unexpected error: %v", e)
				}
			}
			if disabled {
				if opens != 0 || spy.calls != 0 {
					t.Fatal("backend accessed")
				}
			} else if opens != 1 || spy.calls != 4 {
				t.Fatalf("enabled dispatch: opens=%d calls=%d", opens, spy.calls)
			}
		})
	}
}
