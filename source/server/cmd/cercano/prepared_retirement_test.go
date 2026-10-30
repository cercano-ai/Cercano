package main

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/server"
)

func TestPreparedResourcesRetirementContinues(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "error"}[fail], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.db")
			if err := conversation.PrepareCoexistence(t.Context(), path); err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			sentinel := errors.New("disposal sentinel")
			var calls atomic.Int32
			resources := newPreparedShutdown(nil, func() error {
				calls.Add(1)
				close(entered)
				<-release
				if fail {
					return sentinel
				}
				return nil
			})
			s, err := server.NewCoexistenceServer(t.Context(), server.CoexistenceOptions{StorePath: path, Host: "host", Build: "build", Contract: conversation.CurrentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(ctx context.Context) error { return resources.Wait(ctx) }})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			observed := make(chan error, 1)
			go func() { observed <- s.RetireCoexistence(ctx) }()
			<-entered
			cancel()
			if err := <-observed; !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			// Outlive both the observer and the old 100ms disposal timeout.
			time.Sleep(150 * time.Millisecond)
			if resources.Status().Complete {
				t.Fatal("premature completion")
			}
			close(release)
			// No retry drives continuation: resource disposal runs independently.
			select {
			case <-resources.done:
			case <-time.After(time.Second):
				t.Fatal("no automatic continuation")
			}
			err = s.RetireCoexistence(context.Background())
			if fail {
				if !errors.Is(err, sentinel) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if err := s.CloseCoexistenceStore(); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Fatal(calls.Load())
			}
		})
	}
}
