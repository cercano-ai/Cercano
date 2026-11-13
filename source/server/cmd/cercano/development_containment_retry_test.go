package main

import (
	"cercano/source/server/internal/brewrestart"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestPreparedAbsencePermanentError(t *testing.T) {
	cause := errors.New("uncertain kernel identity")
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	calls := 0
	err := waitPreparedWritersAbsent(ctx, func(context.Context) error { calls++; return cause })
	if calls != 1 || !errors.Is(err, cause) || ctx.Err() != nil {
		t.Fatalf("calls=%d err=%v context=%v", calls, err, ctx.Err())
	}
}

func TestPreparedAbsenceRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	err := waitPreparedWritersAbsent(ctx, func(context.Context) error {
		calls++
		if calls == 1 {
			return fmt.Errorf("pid=123: %w", brewrestart.ErrPreparedProcessPresent)
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestPreparedAbsenceRetryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := waitPreparedWritersAbsent(ctx, func(context.Context) error {
		calls++
		cancel()
		return brewrestart.ErrPreparedProcessPresent
	})
	if calls != 1 || !errors.Is(err, context.Canceled) || !errors.Is(err, brewrestart.ErrPreparedProcessPresent) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
