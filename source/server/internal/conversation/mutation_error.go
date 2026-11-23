package conversation

import (
	"errors"
	"strings"
)

// MutationError identifies failure after a store operation reached a mutation
// statement or its commit. Validation, target lookup, and ownership rejection
// are not mutation failures. It preserves the underlying error for errors.Is/As.
// Alternative Store implementations must use it for failed/uncertain writes.
type MutationError struct{ Err error }

func (e *MutationError) Error() string { return e.Err.Error() }
func (e *MutationError) Unwrap() error { return e.Err }

func IsMutationFailure(err error) bool {
	var failure *MutationError
	return errors.As(err, &failure)
}

func mutationError(err error) error {
	if err == nil {
		return nil
	}
	// The transaction-local ownership triggers reject a statement before any
	// mutation, including relationship/descendant checks not covered by the
	// explicit target authorization. Keep these refusals out of write accounting.
	if strings.Contains(err.Error(), "missing or stale conversation ownership capability (including parent/precursor)") {
		return err
	}
	return &MutationError{Err: err}
}
