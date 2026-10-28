package credentials

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"

	"cercano/source/server/internal/filelock"
)

var ErrCoordination = errors.New("credential coordination unavailable or invalid")
var ErrStale = errors.New("credential revision invalidated")
var ErrProviderIntegration = errors.New("coordinated token source integration not yet available")

// RecordBackend is NOT secrets.Store CRUD or a claim of native CAS. Implementers
// must provide fresh, linearizable reads and whole-record durable replacement at
// the EXISTING cercano/profile identity. A successful Replace survives restart.
// A definite failure leaves the prior record intact. An uncertain result MUST
// durably quarantine the identity: Read/Replace must fail across all instances
// and restarts until explicit reconciliation. Never expose possibly-old active
// data after an uncertain write. Keys includes markers. No adapter for the native
// backend is supplied: its replacement/failure guarantees remain unqualified.
// All participants must also use the same stable exclusion root, with legacy
// readers/writers excluded. These obligations cannot be inferred from CRUD.
type RecordBackend interface {
	Read(string) (string, error)
	Replace(string, string) error
	Keys() ([]string, error)
}

type credentialRecord struct {
	Version  int    `json:"version"`
	Revision string `json:"revision"`
	State    string `json:"state"`
	Payload  string `json:"payload,omitempty"`
}

func revision() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Only canonical records emitted here are accepted. This also rejects duplicate
// keys, unknown fields, omitted required fields, trailing data and raw legacy
// secrets; there is deliberately no fallback or adoption path.
func decodeRecord(raw string) (credentialRecord, error) {
	var r credentialRecord
	if json.Unmarshal([]byte(raw), &r) != nil {
		return r, ErrCoordination
	}
	canonical, _ := json.Marshal(r)
	rev, err := hex.DecodeString(r.Revision)
	if !bytes.Equal(canonical, []byte(raw)) || r.Version != 1 || err != nil || len(rev) != 32 {
		return credentialRecord{}, ErrCoordination
	}
	switch r.State {
	case "active":
		if r.Payload == "" {
			return credentialRecord{}, ErrCoordination
		}
	case "logged-out", "pending":
		if r.Payload != "" {
			return credentialRecord{}, ErrCoordination
		}
	default:
		return credentialRecord{}, ErrCoordination
	}
	return r, nil
}

// CoordinatedStore is opt-in only. It does not open native credentials, create
// initial records, adopt raw credentials, or select a production lock location.
// root must be an absolute, private, stable directory shared by all participants
// for this backend authority, never a per-generation config override. Lock files
// are non-secret exclusion only, not a journal. Production preflight is pending.
type CoordinatedStore struct {
	backend RecordBackend
	root    string
}

func NewCoordinatedStore(backend RecordBackend, root string) (*CoordinatedStore, error) {
	if backend == nil || !filepath.IsAbs(root) {
		return nil, ErrCoordination
	}
	return &CoordinatedStore{backend: backend, root: filepath.Clean(root)}, nil
}
func (s *CoordinatedStore) lock(ctx context.Context, name, kind string) (func(), error) {
	sum := sha256.Sum256([]byte("cercano\x00" + name))
	return filelock.Lock(ctx, filepath.Join(s.root, hex.EncodeToString(sum[:])+"."+kind+".lock"))
}
func (s *CoordinatedStore) read(name string) (credentialRecord, error) {
	raw, err := s.backend.Read(name)
	if err != nil {
		return credentialRecord{}, ErrCoordination
	}
	return decodeRecord(raw)
}
func (s *CoordinatedStore) write(name, state, payload string) (credentialRecord, error) {
	rev, err := revision()
	if err != nil {
		return credentialRecord{}, err
	}
	r := credentialRecord{1, rev, state, payload}
	raw, err := json.Marshal(r)
	if err != nil {
		return credentialRecord{}, err
	}
	if _, err = decodeRecord(string(raw)); err != nil {
		return credentialRecord{}, err
	}
	if err = s.backend.Replace(name, string(raw)); err != nil {
		return credentialRecord{}, ErrCoordination
	}
	return r, nil
}
func (s *CoordinatedStore) snapshot(ctx context.Context, name string) (credentialRecord, error) {
	unlock, err := s.lock(ctx, name, "commit")
	if err != nil {
		return credentialRecord{}, err
	}
	defer unlock()
	return s.read(name)
}
func (s *CoordinatedStore) Get(name string) (string, error) {
	r, err := s.snapshot(context.Background(), name)
	if err != nil {
		return "", err
	}
	if r.State != "active" {
		return "", fs.ErrNotExist
	}
	return r.Payload, nil
}
func (s *CoordinatedStore) Set(name, value string) error {
	return s.replace(context.Background(), name, "", "active", value)
}

// Delete's successful durable replacement is the logout boundary. No secret is
// retained in the marker. Missing state is an error, not a reusable generation.
func (s *CoordinatedStore) Delete(name string) error {
	return s.replace(context.Background(), name, "", "logged-out", "")
}
func (s *CoordinatedStore) replace(ctx context.Context, name, expected, state, payload string) error {
	unlock, err := s.lock(ctx, name, "commit")
	if err != nil {
		return err
	}
	defer unlock()
	r, err := s.read(name)
	if err != nil {
		return err
	}
	if expected != "" && r.Revision != expected {
		return ErrStale
	}
	_, err = s.write(name, state, payload)
	return err
}

// List is usable-credential presence, not native item existence. Unlike native
// Keys, it must read records; a backend error fails the entire presence result.
func (s *CoordinatedStore) List() ([]string, error) {
	keys, err := s.backend.Keys()
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, key := range keys {
		r, err := s.snapshot(context.Background(), key)
		if err != nil {
			return nil, err
		}
		if r.State == "active" {
			out = append(out, key)
		}
	}
	return out, nil
}

// Admit is the low-level bounded admission critical section. start must perform
// local bookkeeping only: no network, blocking work, or reusable authorization.
// Admission, not physical send, is the logout boundary. Prefer AdmitAttempt.
func (s *CoordinatedStore) Admit(ctx context.Context, name, expected string, start func() error) error {
	unlock, err := s.lock(ctx, name, "commit")
	if err != nil {
		return err
	}
	defer unlock()
	r, err := s.read(name)
	if err != nil {
		return err
	}
	if r.State != "active" || r.Revision != expected {
		return ErrStale
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return start()
}

// refresh is called only when the source has determined rotation is necessary.
// Separate refresh exclusion spans the fake/real issuer, but commit exclusion
// never does. Pending has no secrets and survives issuer/commit failure. No
// automatic replay or retry is performed. Provider hook integration is pending.
func (s *CoordinatedStore) refresh(ctx context.Context, name, expected string, issuer func(context.Context, string) (string, error)) error {
	release, err := s.lock(ctx, name, "refresh")
	if err != nil {
		return err
	}
	defer release()
	unlock, err := s.lock(ctx, name, "commit")
	if err != nil {
		return err
	}
	r, err := s.read(name)
	if err == nil && (r.State != "active" || r.Revision != expected) {
		err = ErrStale
	}
	var pending credentialRecord
	if err == nil {
		pending, err = s.write(name, "pending", "")
	}
	unlock()
	if err != nil {
		return err
	}
	// Cancellation after pending is conservative uncertainty, never rollback.
	if err = ctx.Err(); err != nil {
		return err
	}
	value, err := issuer(ctx, r.Payload)
	if err != nil {
		return err
	}
	return s.replace(ctx, name, pending.Revision, "active", value)
}
