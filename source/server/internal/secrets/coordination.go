package secrets

import (
	"context"
	"errors"
)

// Coordinated describes revision-fenced login publication. Facades preserve this
// capability without exposing their backing store or bypassing mutation policy.
// IsCoordinated is false for facades over legacy CRUD stores.
type Coordinated interface {
	IsCoordinated() bool
	CredentialRevision(context.Context, string) (string, error)
	SetAtRevision(context.Context, string, string, string) error
}

func IsCoordinated(store Store) bool {
	c, ok := store.(Coordinated)
	return ok && c.IsCoordinated()
}

func (s readOnlyStore) IsCoordinated() bool { return IsCoordinated(s.Store) }
func (s readOnlyStore) CredentialRevision(ctx context.Context, name string) (string, error) {
	if err := CheckAllowed(); err != nil {
		return "", err
	}
	if !IsCoordinated(s.Store) {
		return "", errors.New("credential store is not coordinated")
	}
	return s.Store.(Coordinated).CredentialRevision(ctx, name)
}
func (s readOnlyStore) SetAtRevision(context.Context, string, string, string) error {
	return s.CheckMutation()
}

// CredentialSnapshot binds a payload to the revision read atomically with it.
type CredentialSnapshot struct{ Payload, Revision string }

// RequestCredentials admits exactly one caller-owned request attempt, not a
// reusable client. Success permits later transmission; it does not attest send.
type RequestCredentials interface {
	CredentialSnapshot(context.Context, string) (CredentialSnapshot, error)
	AdmitAttempt(context.Context, string, string) error
}

func (s readOnlyStore) CredentialSnapshot(ctx context.Context, name string) (CredentialSnapshot, error) {
	if err := CheckAllowed(); err != nil {
		return CredentialSnapshot{}, err
	}
	c, ok := s.Store.(RequestCredentials)
	if !ok || !IsCoordinated(s.Store) {
		return CredentialSnapshot{}, errors.New("request admission unavailable")
	}
	return c.CredentialSnapshot(ctx, name)
}
func (s readOnlyStore) AdmitAttempt(ctx context.Context, name, revision string) error {
	if err := CheckAllowed(); err != nil {
		return err
	}
	c, ok := s.Store.(RequestCredentials)
	if !ok || !IsCoordinated(s.Store) {
		return errors.New("request admission unavailable")
	}
	return c.AdmitAttempt(ctx, name, revision)
}
