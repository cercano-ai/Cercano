//go:build !darwin

package enterprise

func OpenCredentialStore(profile string) (CredentialStore, error) { return nil, ErrCredentialStore }
