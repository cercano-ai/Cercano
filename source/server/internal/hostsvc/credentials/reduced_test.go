package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/secrets"
)

// Direct coordinator, memory backend, local issuer only; credential policy stays disabled.
func TestReducedRefreshBusyAndTwoStates(t *testing.T) {
	a, b, back := recordFixture(t)
	r := snapshotTest(t, a)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		done <- a.refresh(ctx, "profile", r.Revision, func(context.Context, string) (string, error) {
			close(entered)
			select {
			case <-release:
				return "new-fake", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
	}()
	<-entered
	defer func() { close(release); <-done }()
	raw, _ := back.Read("profile")
	current, err := decodeRecord(raw)
	if err != nil || current.State != "active" || current.Revision != r.Revision {
		t.Fatalf("refresh changed record before publication: %s %v", current.State, err)
	}
	for name, op := range map[string]func() error{
		"logout":      func() error { return b.Delete("profile") },
		"replacement": func() error { return b.Set("profile", "other-fake") },
		"admission":   func() error { return b.AdmitAttempt(ctx, "profile", r.Revision) },
		"refresh": func() error {
			return b.refresh(ctx, "profile", r.Revision, func(context.Context, string) (string, error) { t.Error("conflicting issuer ran"); return "", nil })
		},
	} {
		if err := op(); !errors.Is(err, ErrBusy) {
			t.Errorf("%s: want busy, got %v", name, err)
		}
	}
}
func TestReducedRejectSecretFreePending(t *testing.T) {
	a, _, back := recordFixture(t)
	r := snapshotTest(t, a)
	raw, _ := json.Marshal(credentialRecord{1, r.Revision, "pending", ""})
	back.data["profile"] = string(raw)
	if _, err := a.Get("profile"); !errors.Is(err, ErrCoordination) {
		t.Fatalf("pending must be refused, got %v", err)
	}
	if err := a.Delete("profile"); !errors.Is(err, ErrCoordination) {
		t.Fatalf("pending migrated: %v", err)
	}
}

// The local service mutex must not hide identity contention behind a wait.
func TestReducedServiceBusyAndGuards(t *testing.T) {
	credentialtest.EnableHermetic(t)
	a, _, _ := recordFixture(t)
	svc := New(a)
	ctx := context.Background()
	snapshot, err := svc.CredentialSnapshot(ctx, "profile")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RefreshSnapshot(ctx, "profile", snapshot.Revision, func(context.Context, string) (string, error) {
		for name, op := range map[string]func() error{
			"logout":      func() error { return svc.Delete("profile") },
			"replacement": func() error { return svc.Set("profile", "fake") },
			"login CAS":   func() error { return svc.SetAtRevision(ctx, "profile", snapshot.Revision, "fake") },
			"admission":   func() error { return svc.AdmitAttempt(ctx, "profile", snapshot.Revision) },
			"refresh": func() error {
				_, e := svc.RefreshSnapshot(ctx, "profile", snapshot.Revision, func(context.Context, string) (string, error) { t.Error("contending issuer ran"); return "", nil })
				return e
			},
		} {
			if e := op(); !errors.Is(e, ErrBusy) {
				t.Errorf("%s: %v", name, e)
			}
		}
		return "fresh-fake", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	readonly := secrets.Guard(svc, secrets.Options{ReadOnly: true}).(secrets.SnapshotRefresher)
	issuer := func(context.Context, string) (string, error) { t.Error("guarded issuer ran"); return "", nil }
	if _, err := readonly.RefreshSnapshot(ctx, "profile", snapshot.Revision, issuer); !errors.Is(err, secrets.ErrReadOnly) {
		t.Fatal(err)
	}
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	if _, err := svc.RefreshSnapshot(ctx, "profile", snapshot.Revision, issuer); !errors.Is(err, secrets.ErrDisabled) {
		t.Fatal(err)
	}
}

func TestReducedRefreshCancellationDoesNotPublish(t *testing.T) {
	a, _, _ := recordFixture(t)
	original := snapshotTest(t, a)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	_, err := a.RefreshSnapshot(ctx, "profile", original.Revision, func(issueCtx context.Context, _ string) (string, error) {
		calls++
		if deadline, ok := issueCtx.Deadline(); !ok || time.Until(deadline) > 30*time.Second {
			t.Error("unbounded issuer context")
		}
		cancel()
		return "unpublishable-fake", nil
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || snapshotTest(t, a) != original {
		t.Fatalf("cancellation: calls=%d error=%v", calls, err)
	}
}
