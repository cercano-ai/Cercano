package enterprise

import "cercano/source/server/internal/secrets"

// OpenCredentialStore rejects credential operations when the shared credential
// gate is disabled or malformed, without entering the platform opener.
func OpenCredentialStore() (CredentialStore, error) {
	return openCredentialStore(openNativeCredentialStore)
}

func openCredentialStore(open func() (CredentialStore, error)) (CredentialStore, error) {
	if err := secrets.CheckAllowed(); err != nil {
		return disabledCredentialStore{err}, nil
	}
	return open()
}

type disabledCredentialStore struct{ err error }

func (s disabledCredentialStore) Get(string) (string, error) { return "", s.err }
func (s disabledCredentialStore) Set(string, string) error   { return s.err }
func (s disabledCredentialStore) Delete(string) error        { return s.err }
