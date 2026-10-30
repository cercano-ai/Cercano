package credentials

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"cercano/source/server/internal/anthropicauth"
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/secrets"
)

func TestAnthropicSubscriptionSnapshot(t *testing.T) {
	credentialtest.EnableHermetic(t)
	forbidden := &credentialtest.ForbiddenTransport{}
	http.DefaultTransport = forbidden
	t.Cleanup(func() {
		if forbidden.Calls.Load() != 0 {
			t.Error("default wire attempted")
		}
	})
	for _, mode := range []string{"fresh", "refresh", "readonly-fresh", "readonly-expired", "logout", "issuer-uncertain", "commit-failure"} {
		t.Run(mode, func(t *testing.T) {
			a, b, backend := recordFixture(t)
			expired := mode != "fresh" && mode != "readonly-fresh"
			seed(t, a, "anthropic", "profile", "old-access", expired)
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
				raw, _ := backend.Read("profile")
				record, decodeErr := decodeRecord(raw)
				if decodeErr != nil || record.State != "active" {
					t.Fatal("issuer changed active record")
				}
				switch mode {
				case "logout":
					// Same-owner logout conflicts with issuer exclusion.
					if err := svc.Delete("profile"); !errors.Is(err, ErrBusy) {
						t.Fatal(err)
					}
				case "issuer-uncertain":
					return nil, errors.New("uncertain issuer")
				case "commit-failure":
					backend.mu.Lock()
					backend.fail = true
					backend.mu.Unlock()
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))}, nil
			})
			source := svc.Anthropic("profile", anthropicauth.Flow{TokenURL: "https://issuer.invalid/token", Client: &http.Client{Transport: issuer}})
			got, err := source.SubscriptionSnapshot(context.Background())
			switch mode {
			case "fresh", "readonly-fresh", "refresh", "logout":
				if err != nil {
					t.Fatal(err)
				}
				stored, err := b.CredentialSnapshot(context.Background(), "profile")
				if err != nil || got != stored {
					t.Fatalf("not committed atomic snapshot: %v", err)
				}
				if mode == "refresh" || mode == "logout" {
					if got.Revision == before.Revision || calls != 1 {
						t.Fatal("rotation missing")
					}

					fresh, _ := anthropicauth.DecodeTokenSet(got.Payload)
					if fresh.Access != "new-access" || fresh.Refresh != "new-refresh" {
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
					// No pending journal or retry: failed issuer/publication leaves
					// the previous active record and reports the operation error.
					after, readErr := b.CredentialSnapshot(context.Background(), "profile")
					if readErr != nil || after != before || calls != 1 {
						t.Fatalf("failure changed record/retried: %v", readErr)
					}

				}
			}
		})
	}
}
