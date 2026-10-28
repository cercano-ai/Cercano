package credentials

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"testing"

	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/secrets"
)

// Diagnostic: legacy CRUD stores cannot fence another Service's staged result.
func TestLegacyTwoServiceResurrectionProbe(t *testing.T) {
	credentialtest.Enable(t)
	raw := secrets.NewMemory()
	if err := raw.Set("work", "old"); err != nil {
		t.Fatal(err)
	}
	a, b := New(raw), New(raw)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	done := make(chan error, 1)
	ctx := testContext(t)
	go func() {
		_, err := a.acquire(ctx, "work", "fake", func(ctx context.Context, tx *transaction) (token, error) {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return token{}, ctx.Err()
			}
			return token{access: "fresh"}, tx.Set("work", "rotated")
		})
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := b.Delete("work"); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Get("work"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("after logout: %v", err)
	}
	unblock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if got, err := raw.Get("work"); err != nil || got != "rotated" {
		t.Fatalf("expected diagnostic resurrection, got %q, %v", got, err)
	}
}
