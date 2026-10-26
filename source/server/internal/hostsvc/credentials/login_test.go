package credentials

import (
	"cercano/source/server/internal/credentialtest"
	"context"
	"errors"
	"testing"

	"cercano/source/server/internal/secrets"
)

func TestLoginNewAttemptOwnsCommit(t *testing.T) {
	credentialtest.Enable(t)
	s := New(secrets.NewMemory())
	first, err := s.BeginLogin(context.Background(), "work", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.BeginLogin(context.Background(), "work", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if first.Context().Err() == nil {
		t.Fatal("superseded login was not canceled")
	}
	if err := second.Commit("new-login"); err != nil {
		t.Fatal(err)
	}
	if err := first.Commit("old-login"); err == nil {
		t.Fatal("obsolete login committed")
	}
	first.Close() // must not undo the later result
	if raw, _ := s.Get("work"); raw != "new-login" {
		t.Fatal("older login overwrote newer credentials")
	}
	if err := second.Commit("duplicate"); err == nil {
		t.Fatal("duplicate commit accepted")
	}
}
func TestLoginCancellationAndReplacement(t *testing.T) {
	credentialtest.Enable(t)
	for _, operation := range []string{"cancel", "close", "write", "delete", "backend"} {
		t.Run(operation, func(t *testing.T) {
			s := New(secrets.NewMemory())
			s.Set("work", "original")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			attempt, err := s.BeginLogin(ctx, "work", "anthropic")
			if err != nil {
				t.Fatal(err)
			}
			defer attempt.Close()
			switch operation {
			case "cancel":
				cancel()
			case "close":
				attempt.Close()
			case "write":
				s.Set("work", "replacement")
			case "delete":
				s.Delete("work")
			case "backend":
				s.ReplaceStore(secrets.NewMemory())
			}
			if err := attempt.Commit("late-login"); err == nil {
				t.Fatal("obsolete login accepted")
			}
			if raw, _ := s.Get("work"); raw == "late-login" {
				t.Fatal("canceled login mutated credentials")
			}
		})
	}
}
func TestClosingOldLoginDoesNotCancelNewOne(t *testing.T) {
	credentialtest.Enable(t)
	s := New(secrets.NewMemory())
	old, err := s.BeginLogin(context.Background(), "work", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.BeginLogin(context.Background(), "work", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	old.Close()
	if err := current.Commit("new"); err != nil {
		t.Fatal(err)
	}
}
func TestDifferentProfilesHaveIndependentLogins(t *testing.T) {
	credentialtest.Enable(t)
	s := New(secrets.NewMemory())
	a, err := s.BeginLogin(context.Background(), "a", "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.BeginLogin(context.Background(), "b", "openai-responses")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	defer b.Close()
	a.Close()
	if err := b.Commit("b-token"); err != nil {
		t.Fatal(err)
	}
}
func TestLoginRejectsCanceledContext(t *testing.T) {
	credentialtest.Enable(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(secrets.NewMemory()).BeginLogin(ctx, "work", "anthropic"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
