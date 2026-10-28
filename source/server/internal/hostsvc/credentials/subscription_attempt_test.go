package credentials

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"cercano/source/server/internal/chatgptauth"
	"cercano/source/server/internal/credentialtest"
)

type subscriptionBlockTransport struct{ calls int }

func (r *subscriptionBlockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls++
	return nil, errors.New("injected issuer must not run")
}

// Reproduce the missing Service integration, including fresh cached material.
// This is a fail-closed characterization, not a claim of subscription support.
func TestSubscriptionAttemptServiceBlocker(t *testing.T) {
	credentialtest.Enable(t)
	for _, expired := range []bool{false, true} {
		name := "fresh"
		if expired {
			name = "expired"
		}
		t.Run(name, func(t *testing.T) {
			store, _, _ := recordFixture(t)
			seed(t, store, "chatgpt", "profile", "fake-access", expired)
			before, err := store.CredentialSnapshot(context.Background(), "profile")
			if err != nil {
				t.Fatal(err)
			}
			transport := &subscriptionBlockTransport{}
			service := New(store)
			source := service.ChatGPT("profile", chatgptauth.Flow{Issuer: "https://issuer.invalid", Client: &http.Client{Transport: transport}})
			for i := 0; i < 2; i++ {
				access, account, err := source.Token(context.Background())
				if !errors.Is(err, ErrProviderIntegration) || access != "" || account != "" {
					t.Fatalf("expected integration refusal without material: %v", err)
				}
			}
			after, err := service.CredentialSnapshot(context.Background(), "profile")
			if err != nil || after != before || transport.calls != 0 {
				t.Fatalf("refusal mutated record or called issuer: %v, calls=%d", err, transport.calls)
			}
		})
	}
}
