package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"
)

// This fixture implements the stronger backend contract, not a native adapter.
// uncertain models durable quarantine shared by every reopened coordinator.
type recordFake struct {
	mu                  sync.Mutex
	data                map[string]string
	fail, uncertain     bool
	uncertainAfterWrite bool
}

func (b *recordFake) Read(k string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.uncertain {
		return "", ErrCoordination
	}
	v, ok := b.data[k]
	if !ok {
		return "", fs.ErrNotExist
	}
	return v, nil
}
func (b *recordFake) Replace(k, v string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fail || b.uncertain {
		return ErrCoordination
	}
	b.data[k] = v
	if b.uncertainAfterWrite {
		// Model a replacement whose acknowledgement was lost. The backend,
		// not the coordinator, must retain quarantine across reopened views.
		b.uncertain = true
		return ErrCoordination
	}
	return nil
}
func (b *recordFake) Keys() ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.uncertain {
		return nil, ErrCoordination
	}
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
func TestCoordinatedLogoutFencesRefreshAndAdmission(t *testing.T) {
	a, b, back := recordFixture(t)
	initial := snapshotTest(t, a)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		done <- a.refresh(ctx, "profile", initial.Revision, func(context.Context, string) (string, error) {
			close(entered)
			select {
			case <-release:
				return "rotated-fake", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if snapshotTest(t, b).State != "pending" {
		t.Fatal("issuer started without pending")
	}
	if err := b.Delete("profile"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrStale) {
		t.Fatalf("late refresh: %v", err)
	}
	if _, err := a.Get("profile"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("logout: %v", err)
	}
	if keys, err := a.List(); err != nil || len(keys) != 0 {
		t.Fatalf("presence: %v %v", keys, err)
	}
	raw, err := back.Read("profile")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "payload") || strings.Contains(raw, "fake") {
		t.Fatal("marker retained secret")
	}
	started := false
	if err := a.Admit(ctx, "profile", initial.Revision, func() error { started = true; return nil }); !errors.Is(err, ErrStale) || started {
		t.Fatalf("stale dispatch: %v %v", err, started)
	}
}
func TestCoordinatedUncertainRefreshNoReplay(t *testing.T) {
	a, b, _ := recordFixture(t)
	r := snapshotTest(t, a)
	calls := 0
	issuer := func(context.Context, string) (string, error) { calls++; return "", errors.New("fake ambiguous issuer") }
	if err := a.refresh(context.Background(), "profile", r.Revision, issuer); err == nil {
		t.Fatal("issuer failure ignored")
	}
	if err := b.refresh(context.Background(), "profile", r.Revision, issuer); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if calls != 1 || snapshotTest(t, b).State != "pending" {
		t.Fatal("replayed uncertain rotation")
	}
	if err := b.Delete("profile"); err != nil {
		t.Fatal(err)
	}
}
func TestCoordinatedBackendFailures(t *testing.T) {
	a, b, back := recordFixture(t)
	r := snapshotTest(t, a)
	back.fail = true
	called := false
	if err := a.refresh(context.Background(), "profile", r.Revision, func(context.Context, string) (string, error) { called = true; return "fake", nil }); err == nil || called {
		t.Fatal("issuer ran without durable pending")
	}
	if err := a.Delete("profile"); err == nil {
		t.Fatal("failed logout acknowledged")
	}
	if snapshotTest(t, b).Revision != r.Revision {
		t.Fatal("definite failure changed record")
	}
	back.fail = false
	if err := a.refresh(context.Background(), "profile", r.Revision, func(context.Context, string) (string, error) { back.fail = true; return "fake", nil }); err == nil {
		t.Fatal("failed publication acknowledged")
	}
	if snapshotTest(t, b).State != "pending" {
		t.Fatal("failed publication lost blocker")
	}
	back.uncertain = true
	if _, err := b.Get("profile"); err == nil {
		t.Fatal("uncertain backend exposed token")
	}
	if _, err := b.List(); err == nil {
		t.Fatal("uncertain backend presence")
	}
}
func TestCoordinatedDispatchBoundary(t *testing.T) {
	a, b, _ := recordFixture(t)
	r := snapshotTest(t, a)
	started, finish := make(chan struct{}), make(chan struct{})
	admitted, loggedOut := make(chan error, 1), make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		admitted <- a.Admit(ctx, "profile", r.Revision, func() error {
			close(started)
			select {
			case <-finish:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() { loggedOut <- b.Delete("profile") }()
	select {
	case err := <-loggedOut:
		t.Fatalf("logout crossed dispatch exclusion: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	close(finish)
	if err := <-admitted; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-loggedOut:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := a.Admit(ctx, "profile", r.Revision, func() error { t.Fatal("dispatch after logout"); return nil }); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}

func TestCoordinatedRefreshSerialization(t *testing.T) {
	a, b, _ := recordFixture(t)
	r := snapshotTest(t, a)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	first, second := make(chan error, 1), make(chan error, 1)
	go func() {
		first <- a.refresh(ctx, "profile", r.Revision, func(context.Context, string) (string, error) {
			close(entered)
			select {
			case <-release:
				return "rotated-fake", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go func() {
		second <- b.refresh(ctx, "profile", r.Revision, func(context.Context, string) (string, error) { return "", errors.New("second issuer must not run") })
	}()
	select {
	case err := <-second:
		t.Fatalf("refresh exclusion not held: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if got, err := b.Get("profile"); err != nil || got != "rotated-fake" {
		t.Fatal("publication unavailable", err)
	}
}

// This tests the coordinator's response to a qualified backend's quarantine,
// not native crash durability (the fixture is intentionally memory-only).
func TestCoordinatedUncertainReplacementReopenedView(t *testing.T) {
	a, b, back := recordFixture(t)
	r := snapshotTest(t, a)
	back.uncertainAfterWrite = true
	if err := a.Delete("profile"); !errors.Is(err, ErrCoordination) {
		t.Fatalf("uncertain logout acknowledged: %v", err)
	}
	reopened, err := NewCoordinatedStore(back, a.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []*CoordinatedStore{a, b, reopened} {
		if _, err := view.Get("profile"); !errors.Is(err, ErrCoordination) {
			t.Fatal("quarantine bypass", err)
		}
		if _, err := view.List(); err == nil {
			t.Fatal("uncertain presence")
		}
		if err := view.Set("profile", "fake-new"); err == nil {
			t.Fatal("quarantine overwritten")
		}
		if err := view.Admit(context.Background(), "profile", r.Revision, func() error { t.Fatal("uncertain dispatch"); return nil }); err == nil {
			t.Fatal("uncertain admission")
		}
	}
}
