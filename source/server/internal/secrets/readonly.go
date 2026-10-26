package secrets

import "errors"

// ErrReadOnly identifies a fixed coexistence shared-mutation freeze. Unlike
// disabled credentials, this mode permits reads for ordinary inference.
var ErrReadOnly = errors.New("shared credentials/config are read-only in coexistence mode")

// Options is copied at construction; never derive it from active client counts.
type Options struct{ ReadOnly bool }

func (o Options) CheckMutation() error {
	if err := CheckAllowed(); err != nil {
		return err
	}
	if o.ReadOnly {
		return ErrReadOnly
	}
	return nil
}

// CheckMutation also recognizes guarded stores, including narrow auth stores.
func CheckMutation(store any) error {
	if err := CheckAllowed(); err != nil {
		return err
	}
	if s, ok := store.(interface{ CheckMutation() error }); ok {
		return s.CheckMutation()
	}
	return nil
}

// Guard captures a fixed read-only boundary without disabling credential reads.
func Guard(store Store, options Options) Store {
	if !options.ReadOnly {
		return store
	}
	return readOnlyStore{Store: store}
}

type readOnlyStore struct{ Store }

func (readOnlyStore) CheckMutation() error       { return Options{ReadOnly: true}.CheckMutation() }
func (s readOnlyStore) Set(string, string) error { return s.CheckMutation() }
func (s readOnlyStore) Delete(string) error      { return s.CheckMutation() }
