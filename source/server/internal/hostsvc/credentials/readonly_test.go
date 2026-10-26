package credentials_test

import (
	"cercano/source/server/internal/credentialtest"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"cercano/source/server/internal/anthropicauth"
	"cercano/source/server/internal/chatgptauth"
	"cercano/source/server/internal/hostsvc/credentials"
	"cercano/source/server/internal/secrets"
)

type countedStore struct {
	raw    string
	writes int
}

func (s *countedStore) Get(string) (string, error) { return s.raw, nil }
func (s *countedStore) List() ([]string, error)    { return []string{"p"}, nil }
func (s *countedStore) Set(string, string) error   { s.writes++; return nil }
func (s *countedStore) Delete(string) error        { s.writes++; return nil }

type rejectTransport struct{ calls int }

func (r *rejectTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls++
	return nil, errors.New("unexpected network")
}

func TestReadOnlyBoundaries(t *testing.T) {
	credentialtest.Enable(t)
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	ctx := context.Background()
	options := secrets.Options{ReadOnly: true}
	raw := &countedStore{raw: "api-key"}
	guarded := secrets.Guard(raw, options)
	svc := credentials.New(raw, options)
	for _, err := range []error{anthropicauth.Save(guarded, "p", anthropicauth.TokenSet{Access: "new"}), chatgptauth.Save(guarded, "p", chatgptauth.TokenSet{Access: "new"})} {
		if !errors.Is(err, secrets.ErrReadOnly) {
			t.Fatalf("Save: %v", err)
		}
	}
	for _, store := range []secrets.Store{guarded, svc} {
		if got, err := store.Get("p"); err != nil || got != "api-key" {
			t.Fatalf("read: %q %v", got, err)
		}
		if _, err := store.List(); err != nil {
			t.Fatal(err)
		}
		for _, err := range []error{store.Set("p", "new"), store.Delete("p")} {
			if !errors.Is(err, secrets.ErrReadOnly) {
				t.Fatalf("mutation: %v", err)
			}
		}
	}
	if _, err := svc.BeginLogin(ctx, "p", "anthropic"); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
	replacement := &countedStore{raw: "replacement"}
	svc.ReplaceStore(replacement)
	if err := svc.Set("p", "new"); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
	if raw.writes != 0 || replacement.writes != 0 {
		t.Fatal("backend mutation")
	}
}

func TestReadOnlyOAuth(t *testing.T) {
	credentialtest.Enable(t)
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	ctx := context.Background()
	options := secrets.Options{ReadOnly: true}
	transport := &rejectTransport{}
	client := &http.Client{Transport: transport}
	af := anthropicauth.Flow{Client: client}
	cf := chatgptauth.Flow{Client: client}
	for _, expired := range []bool{false, true} {
		expiry := time.Now().Add(time.Hour)
		if expired {
			expiry = time.Now().Add(-time.Hour)
		}
		a, _ := (anthropicauth.TokenSet{Access: "access", Refresh: "refresh", ExpiresAt: expiry}).Encode()
		c, _ := (chatgptauth.TokenSet{Access: "access", Refresh: "refresh", ExpiresAt: expiry}).Encode()
		for _, host := range []bool{false, true} {
			as, cs := &countedStore{raw: a}, &countedStore{raw: c}
			var av, cv string
			var ae, ce error
			if host {
				av, ae = credentials.New(as, options).Anthropic("p", af).Token(ctx)
				cv, _, ce = credentials.New(cs, options).ChatGPT("p", cf).Token(ctx)
			} else {
				av, ae = anthropicauth.NewSource(as, "p", af, options).Token(ctx)
				cv, _, ce = chatgptauth.NewSource(cs, "p", cf, options).Token(ctx)
			}
			if expired {
				if !errors.Is(ae, secrets.ErrReadOnly) || !errors.Is(ce, secrets.ErrReadOnly) {
					t.Fatalf("refresh: %v %v", ae, ce)
				}
			} else if ae != nil || ce != nil || av != "access" || cv != "access" {
				t.Fatalf("fresh: %q %q %v %v", av, cv, ae, ce)
			}
			if as.writes != 0 || cs.writes != 0 {
				t.Fatal("refresh wrote backend")
			}
		}
	}
	af.Options, cf.Options = options, options
	if _, err := af.Start(ctx); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
	if _, err := cf.Start(ctx); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
	if _, err := af.Refresh(ctx, "refresh"); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
	if _, err := cf.Refresh(ctx, "refresh"); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
	if transport.calls != 0 {
		t.Fatalf("network calls: %d", transport.calls)
	}
}
