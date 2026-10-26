//go:build !darwin

package enterprise

func openNativeCredentialStore() (CredentialStore, error) { return nil, ErrCredentialStore }
