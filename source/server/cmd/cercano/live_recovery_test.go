package main

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"testing"
)

func TestLiveRecoveryRefusesUnqualifiedWriters(t *testing.T) {
	p := newDevelopmentContainedSource(t.TempDir(), &pb.LifecycleIdentity{Host: "destination"})
	called := false
	err := p.WithStoppedSource(context.Background(), &pb.LifecycleIdentity{Host: "source"}, "snapshot", func() error { called = true; return nil }, func(context.Context) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("unqualified writer graph admitted")
	}
}
