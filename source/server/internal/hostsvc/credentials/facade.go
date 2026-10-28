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
