package credentials

import (
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/secrets"
	"context"
	"errors"
	"io/fs"
	"testing"
)

func TestCoordinatedServiceLoginAndReplacement(t *testing.T) {
	// No parallel tests or children; both services use only this explicit fake.
	credentialtest.Enable(t)
	a, b, _ := recordFixture(t)
	first, second := New(a), New(b)
	login, err := first.BeginLogin(context.Background(), "profile", "fake")
	if err != nil {
		t.Fatal(err)
	}
	defer login.Close()
	if err := second.Delete("profile"); err != nil {
		t.Fatal(err)
	}
	if err := login.Commit("late-fake"); !errors.Is(err, ErrStale) {
		t.Fatalf("stale login: %v", err)
	}
	if _, err := first.Get("profile"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if keys, err := first.List(); err != nil || len(keys) != 0 {
		t.Fatal(keys, err)
	}
	next, err := first.BeginLogin(context.Background(), "profile", "fake")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err := next.Commit("fresh-fake"); err != nil {
		t.Fatal(err)
	}
	if got, err := second.Get("profile"); err != nil || got != "fresh-fake" {
		t.Fatal("fresh login unavailable", err)
	}
	if err := first.ReplaceStore(secrets.NewMemory()); !errors.Is(err, ErrCoordination) {
		t.Fatal("downgrade allowed", err)
	}
	if err := first.ReplaceStore(b); !errors.Is(err, ErrCoordination) {
		t.Fatal("rebind allowed", err)
	}
	if err := first.ReplaceStore(a); err != nil {
		t.Fatal(err)
	}
	invoked := false
	if _, err := first.acquire(context.Background(), "profile", "fake", func(context.Context, *transaction) (token, error) { invoked = true; return token{}, nil }); !errors.Is(err, ErrProviderIntegration) || invoked {
		t.Fatal("legacy source bypass", err)
	}
}
