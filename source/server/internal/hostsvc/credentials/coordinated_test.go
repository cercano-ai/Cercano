package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
)

// Memory-only whole-record backend; no native or recovery guarantees.
type recordFake struct {
	mu   sync.Mutex
	data map[string]string
	fail bool
}

func (b *recordFake) Read(k string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.data[k]
	if !ok {
		return "", fs.ErrNotExist
	}
	return v, nil
}
func (b *recordFake) Replace(k, v string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fail {
		return ErrCoordination
	}
	b.data[k] = v
	return nil
}
func (b *recordFake) Keys() ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []string{}
	for k := range b.data {
		out = append(out, k)
	}
	return out, nil
}
func recordFixture(t *testing.T) (*CoordinatedStore, *CoordinatedStore, *recordFake) {
	t.Helper()
	rev, err := revision()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(credentialRecord{1, rev, "active", "fake-secret"})
	b := &recordFake{data: map[string]string{"profile": string(raw)}}
	root := t.TempDir()
	a, err := NewCoordinatedStore(b, root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewCoordinatedStore(b, root)
	if err != nil {
		t.Fatal(err)
	}
	return a, c, b
}
func snapshotTest(t *testing.T, s *CoordinatedStore) credentialRecord {
	t.Helper()
	r, err := s.snapshot(context.Background(), "profile")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCoordinatedStrictDecode(t *testing.T) {
	a, _, b := recordFixture(t)
	valid := b.data["profile"]
	cases := []string{"raw-api-key", "{}", valid + "{}", strings.Replace(valid, "\"version\":1", "\"version\":2", 1), strings.Replace(valid, "\"version\":1", "\"version\":1,\"version\":1", 1), strings.Replace(valid, "\"version\":1", "\"version\":1,\"unknown\":0", 1), strings.Replace(valid, "active", "logged-out", 1), strings.Replace(valid, "active", "pending", 1), strings.Replace(valid, "active", "unknown", 1)}
	for _, raw := range cases {
		b.data["profile"] = raw
		if _, err := a.Get("profile"); err == nil {
			t.Fatal("invalid record accepted")
		}
		if err := a.Set("profile", "replacement"); err == nil {
			t.Fatal("implicit adoption")
		}
	}
	delete(b.data, "profile")
	if err := a.Set("profile", "replacement"); err == nil {
		t.Fatal("missing record initialized")
	}
}
func TestCoordinatedPublicationAndLogout(t *testing.T) {
	a, b, back := recordFixture(t)
	initial := snapshotTest(t, a)
	if err := a.refresh(context.Background(), "profile", initial.Revision, func(context.Context, string) (string, error) { return "rotated-fake", nil }); err != nil {
		t.Fatal(err)
	}
	if err := b.AdmitAttempt(context.Background(), "profile", initial.Revision); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if err := b.Delete("profile"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Get("profile"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if keys, err := a.List(); err != nil || len(keys) != 0 {
		t.Fatalf("presence: %v %v", keys, err)
	}
	raw, _ := back.Read("profile")
	if strings.Contains(raw, "payload") || strings.Contains(raw, "fake") {
		t.Fatal("marker retained secret")
	}
	if err := a.AdmitAttempt(context.Background(), "profile", initial.Revision); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}

func TestCoordinatedFailuresDoNotJournalOrRetry(t *testing.T) {
	a, b, back := recordFixture(t)
	r := snapshotTest(t, a)
	calls := 0
	if err := a.refresh(context.Background(), "profile", r.Revision, func(context.Context, string) (string, error) { calls++; return "", errors.New("fake issuer failure") }); err == nil {
		t.Fatal("issuer failure ignored")
	}
	if calls != 1 || snapshotTest(t, b) != r {
		t.Fatal("failure changed record or retried")
	}
	back.fail = true
	if err := a.Delete("profile"); err == nil {
		t.Fatal("failed logout acknowledged")
	}
	if err := a.refresh(context.Background(), "profile", r.Revision, func(context.Context, string) (string, error) { calls++; return "fake", nil }); err == nil {
		t.Fatal("failed publication acknowledged")
	}
	if calls != 2 || snapshotTest(t, b) != r {
		t.Fatal("publication failure changed record or retried")
	}
}

func TestCoordinatedDispatchBoundary(t *testing.T) {
	a, b, _ := recordFixture(t)
	r := snapshotTest(t, a)
	if err := a.Admit(context.Background(), "profile", r.Revision, func() error {
		if err := b.Delete("profile"); !errors.Is(err, ErrBusy) {
			t.Fatalf("logout while admitted: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete("profile"); err != nil {
		t.Fatal(err)
	}
	if err := a.AdmitAttempt(context.Background(), "profile", r.Revision); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}
