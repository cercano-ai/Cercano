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
