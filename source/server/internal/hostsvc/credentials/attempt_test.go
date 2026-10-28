package credentials

import (
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/secrets"
	"context"
	"errors"
	"testing"
)

func TestAttemptReadonlySnapshotLogout(t *testing.T) {
	// Every dependency is memory/fake; no fallback, issuer, or child process.
	credentialtest.Enable(t)
	a, b, _ := recordFixture(t)
	owner := New(a)
	view := secrets.Guard(owner, secrets.Options{ReadOnly: true})
	requests := view.(secrets.RequestCredentials)
	ctx := context.Background()
	snap, err := requests.CredentialSnapshot(ctx, "profile")
	if err != nil || snap.Payload != "fake-secret" || snap.Revision == "" {
		t.Fatal(snap.Revision, err)
	}
	if err := requests.AdmitAttempt(ctx, "profile", snap.Revision); err != nil {
		t.Fatal(err)
	}
	// Admission returned: no network lifetime holds the exclusion. An independent
	// owner can commit logout while the already admitted request remains pending.
	if err := b.Delete("profile"); err != nil {
		t.Fatal(err)
	}
	if err := requests.AdmitAttempt(ctx, "profile", snap.Revision); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if _, err := requests.CredentialSnapshot(ctx, "profile"); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if err := view.Set("profile", "other"); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
	if err := view.Delete("profile"); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
}
