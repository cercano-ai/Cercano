package credentials

import (
	"cercano/source/server/internal/secrets"
	"context"
)

func (*CoordinatedStore) IsCoordinated() bool { return true }
func (s *CoordinatedStore) CredentialRevision(ctx context.Context, name string) (string, error) {
	r, err := s.snapshot(ctx, name)
	return r.Revision, err
}
func (s *CoordinatedStore) SetAtRevision(ctx context.Context, name, revision, value string) error {
	return s.replace(ctx, name, revision, "active", value)
}

func (s *Service) IsCoordinated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return secrets.IsCoordinated(s.store)
}
func (s *Service) CredentialRevision(ctx context.Context, name string) (string, error) {
	if err := secrets.CheckAllowed(); err != nil {
		return "", err
	}
	p := s.profile(name)
	p.mu.Lock()
	defer p.mu.Unlock()
	if !secrets.IsCoordinated(p.store) {
		return "", ErrCoordination
	}
	return p.store.(secrets.Coordinated).CredentialRevision(ctx, name)
}
func (s *Service) SetAtRevision(ctx context.Context, name, revision, value string) error {
	if err := s.CheckMutation(); err != nil {
		return err
	}
	p := s.profile(name)
	p.mu.Lock()
	defer p.mu.Unlock()
	if !secrets.IsCoordinated(p.store) {
		return ErrCoordination
	}
	if err := p.store.(secrets.Coordinated).SetAtRevision(ctx, name, revision, value); err != nil {
		return err
	}
	p.invalidate()
	return nil
}

func (s *CoordinatedStore) CredentialSnapshot(ctx context.Context, name string) (secrets.CredentialSnapshot, error) {
	r, err := s.snapshot(ctx, name)
	if err != nil {
		return secrets.CredentialSnapshot{}, err
	}
	if r.State != "active" {
		return secrets.CredentialSnapshot{}, ErrStale
	}
	return secrets.CredentialSnapshot{Payload: r.Payload, Revision: r.Revision}, nil
}

// AdmitAttempt linearizes admission against logout, never network dispatch.
func (s *CoordinatedStore) AdmitAttempt(ctx context.Context, name, revision string) error {
	return s.Admit(ctx, name, revision, func() error { return nil })
}
func (s *Service) CredentialSnapshot(ctx context.Context, name string) (secrets.CredentialSnapshot, error) {
	if err := secrets.CheckAllowed(); err != nil {
		return secrets.CredentialSnapshot{}, err
	}
	p := s.profile(name)
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.store.(secrets.RequestCredentials)
	if !ok || !secrets.IsCoordinated(p.store) {
		return secrets.CredentialSnapshot{}, ErrCoordination
	}
	return c.CredentialSnapshot(ctx, name)
}

// RefreshSnapshot deliberately releases the local profile mutex before issuer
// work so conflicting local operations reach the identity lock and return busy.
// Coordinated authorities cannot be rebound.
func (s *Service) RefreshSnapshot(ctx context.Context, name, revision string, issuer func(context.Context, string) (string, error)) (secrets.CredentialSnapshot, error) {
	if err := s.CheckMutation(); err != nil {
		return secrets.CredentialSnapshot{}, err
	}
	p := s.profile(name)
	p.mu.Lock()
	c, ok := p.store.(secrets.SnapshotRefresher)
	coordinated := secrets.IsCoordinated(p.store)
	p.mu.Unlock()
	if !ok || !coordinated {
		return secrets.CredentialSnapshot{}, ErrCoordination
	}
	return c.RefreshSnapshot(ctx, name, revision, issuer)
}

func (s *Service) AdmitAttempt(ctx context.Context, name, revision string) error {
	if err := secrets.CheckAllowed(); err != nil {
		return err
	}
	p := s.profile(name)
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.store.(secrets.RequestCredentials)
	if !ok || !secrets.IsCoordinated(p.store) {
		return ErrCoordination
	}
	return c.AdmitAttempt(ctx, name, revision)
}
