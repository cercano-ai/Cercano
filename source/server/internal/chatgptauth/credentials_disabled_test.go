package chatgptauth

import (
	"cercano/source/server/internal/secrets"
	"context"
	"errors"
	"testing"
)

// Nil dependencies prove admission precedes store, cached-token, and network use.
func TestCredentialAdmissionBeforeDependencies(t *testing.T) {
	for _, value := range []string{"1", "malformed"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(secrets.DisableEnv, value)
			ctx := context.Background()
			check := func(err error) {
				t.Helper()
				if !errors.Is(err, secrets.ErrDisabled) {
					t.Fatalf("got %v", err)
				}
			}
			_, _, err := (*Source)(nil).Token(ctx)
			check(err)
			check(Save(nil, "profile", TokenSet{}))
			_, err = (Flow{}).Start(ctx)
			check(err)
			_, err = (Flow{}).Refresh(ctx, "fixture")
			check(err)
			_, err = (*Pending)(nil).Poll(ctx)
			check(err)
		})
	}
}
