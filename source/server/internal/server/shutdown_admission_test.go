package server

import (
	"context"
	"testing"
)

func TestShutdownAdmissionWatcherSelectedEvent(t *testing.T) {
	s := &Server{}
	s.FenceBackgroundWork()
	// Simulate a ready fsnotify event winning select after cancellation.
	// No config service is installed: reaching disk reload would panic.
	s.reloadConfigFromDisk(context.Background())
	s.WaitForWatchers()
}
func TestShutdownAdmissionEnteredWatcherJoin(t *testing.T) {
	s := &Server{}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() { s.runWatchCallback(func() { close(entered); <-release }); close(done) }()
	<-entered
	s.FenceBackgroundWork()
	s.runWatchCallback(func() { t.Error("new callback entered") })
	joined := make(chan struct{})
	go func() { s.WaitForWatchers(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("joined active watcher")
	default:
	}
	sibling := &Server{}
	ran := false
	sibling.runWatchCallback(func() { ran = true })
	if !ran {
		t.Fatal("sibling fenced")
	}
	close(release)
	<-done
	<-joined
}
