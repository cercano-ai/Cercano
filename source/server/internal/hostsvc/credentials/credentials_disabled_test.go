package credentials

import (
	"cercano/source/server/internal/secrets"
	"context"
	"errors"
	"testing"
)

func TestCredentialAdmissionBeforeDependencies(t *testing.T) {
	for _, value := range []string{"1", "malformed"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(secrets.DisableEnv, value)
			var s *Service
			check := func(err error) {
				t.Helper()
				if !errors.Is(err, secrets.ErrDisabled) {
					t.Fatalf("got %v", err)
				}
			}
			_, err := s.Get("p")
			check(err)
			check(s.Set("p", "fixture"))
			check(s.Delete("p"))
			_, err = s.List()
			check(err)
			_, err = s.BeginLogin(context.Background(), "p", "anthropic")
			check(err)
			check((*LoginAttempt)(nil).Commit("fixture"))
			_, err = s.acquire(context.Background(), "p", "anthropic", nil)
			check(err)
		})
	}
}
