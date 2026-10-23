package secrets

import (
	"errors"
	"fmt"
	"os"
)

const DisableEnv = "CERCANO_DISABLE_CREDENTIALS"

var ErrDisabled = errors.New("credentials disabled by CERCANO_DISABLE_CREDENTIALS")

// ValidateEnvironment accepts only an absent setting or the explicit values 0
// and 1. Empty and malformed values fail closed, never opening a backend.
func ValidateEnvironment() error {
	v, present := os.LookupEnv(DisableEnv)
	if present && v != "0" && v != "1" {
		return fmt.Errorf("%w: expected 0 or 1", ErrDisabled)
	}
	return nil
}

// CheckAllowed is the credential boundary, including inline keys and OAuth.
// It does not inspect HOME, configuration, or any credential backend.
func CheckAllowed() error {
	if err := ValidateEnvironment(); err != nil {
		return err
	}
	if os.Getenv(DisableEnv) == "1" {
		return ErrDisabled
	}
	return nil
}

type disabledStore struct{ err error }

func (s disabledStore) Get(string) (string, error) { return "", s.err }
func (s disabledStore) Set(string, string) error   { return s.err }
func (s disabledStore) Delete(string) error        { return s.err }
func (s disabledStore) List() ([]string, error)    { return nil, s.err }
