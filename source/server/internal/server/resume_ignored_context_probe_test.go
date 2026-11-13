package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

// This diagnostic records the pre-fix failure without leaking a callback: the
// callback is released and joined even when it ignores the expired context.
func TestBoundedPreflightIgnoredContextDiagnostic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := boundedPreflight(ctx, func(context.Context) (int, error) {
			close(entered)
			<-release
			return 1, nil
		})
		done <- err
	}()
	<-entered
	<-ctx.Done()
	select {
	case err := <-done:
		close(release)
		t.Fatalf("diagnostic no longer reproduces: preflight returned without callback release: %v", err)
	case <-time.After(100 * time.Millisecond):
		t.Log("reproduced: preflight still waits after caller deadline while callback ignores context")
	}
	close(release)
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("post-release error = %v, want deadline exceeded", err)
	}
}
