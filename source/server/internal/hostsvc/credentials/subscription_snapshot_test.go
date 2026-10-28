package credentials

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"cercano/source/server/internal/chatgptauth"
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/secrets"
)

type snapshotIssuer func(*http.Request) (*http.Response, error)

func (f snapshotIssuer) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSubscriptionSnapshot(t *testing.T) {
	credentialtest.Enable(t)
	for _, mode := range []string{"fresh", "refresh", "readonly-fresh", "readonly-expired", "logout", "issuer-uncertain", "commit-uncertain"} {
		t.Run(mode, func(t *testing.T) {
			a, b, backend := recordFixture(t)
			expired := mode != "fresh" && mode != "readonly-fresh"
			seed(t, a, "chatgpt", "profile", "old-access", expired)
			before, err := a.CredentialSnapshot(context.Background(), "profile")
			if err != nil {
				t.Fatal(err)
			}
			var store secrets.Store = a
			if strings.HasPrefix(mode, "readonly-") {
				store = secrets.Guard(store, secrets.Options{ReadOnly: true})
			}
			svc := New(store)
			calls := 0
			issuer := snapshotIssuer(func(*http.Request) (*http.Response, error) {
				calls++
				if snapshotTest(t, b).State != "pending" {
					t.Fatal("issuer without pending")
				}
				switch mode {
				case "logout":
					// Same-owner logout must not block behind issuer work.
					if err := svc.Delete("profile"); err != nil {
						t.Fatal(err)
					}
				case "issuer-uncertain":
					return nil, errors.New("uncertain issuer")
				case "commit-uncertain":
					backend.mu.Lock()
					backend.uncertainAfterWrite = true
					backend.mu.Unlock()
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))}, nil
			})
			source := svc.ChatGPT("profile", chatgptauth.Flow{Issuer: "https://issuer.invalid", Client: &http.Client{Transport: issuer}})
			got, err := source.SubscriptionSnapshot(context.Background())
			switch mode {
			case "fresh", "readonly-fresh", "refresh":
				if err != nil {
					t.Fatal(err)
				}
				stored, err := b.CredentialSnapshot(context.Background(), "profile")
				if err != nil || got != stored {
					t.Fatalf("not committed atomic snapshot: %v", err)
				}
				if mode == "refresh" {
					if got.Revision == before.Revision || calls != 1 {
						t.Fatal("rotation missing")
					}
					old, _ := chatgptauth.DecodeTokenSet(before.Payload)
					fresh, _ := chatgptauth.DecodeTokenSet(got.Payload)
					if fresh.Access != "new-access" || fresh.AccountID != old.AccountID {
						t.Fatal("incorrect refreshed payload")
					}
				} else if got != before || calls != 0 {
					t.Fatal("fresh path rotated")
				}
				if err := svc.AdmitAttempt(context.Background(), "profile", got.Revision); err != nil {
					t.Fatalf("fresh admission: %v", err)
				}
				if err := b.Delete("profile"); err != nil {
					t.Fatal(err)
				}
				if err := svc.AdmitAttempt(context.Background(), "profile", got.Revision); !errors.Is(err, ErrStale) {
					t.Fatalf("stale admission: %v", err)
				}
			default:
				if err == nil || got != (secrets.CredentialSnapshot{}) {
					t.Fatal("failed rotation exposed material")
				}
				if mode == "readonly-expired" {
					if !errors.Is(err, secrets.ErrReadOnly) || calls != 0 {
						t.Fatalf("readonly refresh: %v", err)
					}
					after, _ := a.CredentialSnapshot(context.Background(), "profile")
					if after != before {
						t.Fatal("readonly mutation")
					}
				} else {
					if mode == "commit-uncertain" {
						if _, err := b.CredentialSnapshot(context.Background(), "profile"); err == nil {
							t.Fatal("uncertain commit escaped quarantine")
						}
					} else {
						wantState := "pending"
						if mode == "logout" {
							wantState = "logged-out"
						}
						if snapshotTest(t, b).State != wantState {
							t.Fatal("failed rotation lost blocker")
						}
					}
					if _, err := source.SubscriptionSnapshot(context.Background()); err == nil {
						t.Fatal("failed rotation replayed")
					}
					if calls != 1 {
						t.Fatal("issuer automatically replayed")
					}
				}
			}
		})
	}
}
