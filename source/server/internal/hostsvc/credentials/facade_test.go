package credentials

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/secrets"
)

func TestNestedServiceStaleLogin(t *testing.T) {
	credentialtest.Enable(t)
	a, b, _ := recordFixture(t)
	outer, other := New(New(a)), New(b)
	login, err := outer.BeginLogin(context.Background(), "profile", "fake")
	if err != nil {
		t.Fatal(err)
	}
	defer login.Close()
	if err := other.Delete("profile"); err != nil {
		t.Fatal(err)
	}
	if err := login.Commit("late"); !errors.Is(err, ErrStale) {
		t.Fatalf("late commit: %v", err)
	}
	if _, err := outer.Get("profile"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestGuardedCoordinatedAcquisitionRefused(t *testing.T) {
	credentialtest.Enable(t)
	a, _, _ := recordFixture(t)
	s := New(secrets.Guard(New(a), secrets.Options{ReadOnly: true}))
	invoked := false
	_, err := s.acquire(context.Background(), "profile", "fake", func(context.Context, *transaction) (token, error) { invoked = true; return token{}, nil })
	if !errors.Is(err, ErrProviderIntegration) || invoked {
		t.Fatalf("acquisition: %v, invoked=%v", err, invoked)
	}
	if _, err := s.BeginLogin(context.Background(), "profile", "fake"); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
}

func TestFacadeReplacementCannotDowngradeOrRebind(t *testing.T) {
	credentialtest.Enable(t)
	a, b, _ := recordFixture(t)
	for _, wrapped := range []secrets.Store{New(a), secrets.Guard(a, secrets.Options{ReadOnly: true})} {
		s := New(wrapped)
		for _, replacement := range []secrets.Store{secrets.NewMemory(), New(b), secrets.Guard(b, secrets.Options{ReadOnly: true})} {
			if err := s.ReplaceStore(replacement); !errors.Is(err, ErrCoordination) {
				t.Fatalf("replacement %T: %v", replacement, err)
			}
		}
		if err := s.ReplaceStore(wrapped); err != nil {
			t.Fatal(err)
		}
	}
	if err := New(secrets.NewMemory()).ReplaceStore(New(a)); !errors.Is(err, ErrCoordination) {
		t.Fatal("coordinated upgrade", err)
	}
}

func TestFacadePolicyAndStrictRecord(t *testing.T) {
	credentialtest.Enable(t)
	a, _, back := recordFixture(t)
	ctx := context.Background()
	guarded := secrets.Guard(a, secrets.Options{ReadOnly: true}).(secrets.Coordinated)
	revision, err := guarded.CredentialRevision(ctx, "profile")
	if err != nil {
		t.Fatal(err)
	}
	if err := guarded.SetAtRevision(ctx, "profile", revision, "blocked"); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
	s := New(New(a))
	login, err := s.BeginLogin(ctx, "profile", "fake")
	if err != nil {
		t.Fatal(err)
	}
	defer login.Close()
	if err := login.Commit("fresh"); err != nil {
		t.Fatal(err)
	}
	back.data["profile"] = `{"version":99}`
	if _, err := s.BeginLogin(ctx, "profile", "fake"); err == nil {
		t.Fatal("unknown record admitted")
	}
	if err := s.Set("profile", "raw"); err == nil {
		t.Fatal("unknown record overwritten")
	}
}

func TestLegacyFacadeStillUsesLegacyAcquisition(t *testing.T) {
	credentialtest.Enable(t)
	s := New(New(secrets.NewMemory()))
	if err := s.Set("profile", "old"); err != nil {
		t.Fatal(err)
	}
	value, err := s.acquire(context.Background(), "profile", "fake", func(_ context.Context, tx *transaction) (token, error) {
		if err := tx.Set("profile", "new"); err != nil {
			return token{}, err
		}
		return token{access: "new"}, nil
	})
	if err != nil || value.access != "new" {
		t.Fatal(value, err)
	}
	if got, err := s.Get("profile"); err != nil || got != "new" {
		t.Fatal(got, err)
	}
	if err := s.ReplaceStore(secrets.NewMemory()); err != nil {
		t.Fatal(err)
	}
}
