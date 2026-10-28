package server

import (
	"cercano/source/server/internal/credentialtest"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherSnapshotAdoptsWithoutSaving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	base, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.SaveSnapshot(base, path, base.Snapshot()); err != nil {
		t.Fatal(err)
	}
	initial, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := newConfigWatcherTestServer(t)
	s.SetConfigPersistence(path, initial)
	data, _ := os.ReadFile(path)
	data = append(data, []byte("\n# external comment preserved\n")...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s.reloadConfigFromDisk(context.Background())
	authoritative, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.cfgSvc.Get().Snapshot().Revision != authoritative.Snapshot().Revision {
		t.Fatal("watcher failed to adopt no-diff baseline")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(data) {
		t.Fatal("watcher resaved authority")
	}
	s.reloadConfigFromDisk(context.Background())
	draft := s.cfgSvc.Get()
	draft.Port = "12456"
	if err := s.cfgSvc.SaveAndSet(draft); err != nil {
		t.Fatalf("adopted baseline unusable: %v", err)
	}
}

type recordingReconfigure struct {
	providers.ComposableResolver
	endpoint string
}

func (r *recordingReconfigure) Reconfigure(a providers.ReconfigureArgs) { r.endpoint = a.OllamaURL }

type pausedSnapshotService struct {
	cfgsvc.Service
	saved  chan struct{}
	resume chan struct{}
}

func (p *pausedSnapshotService) SaveAndSet(c config.Config) error {
	if err := p.Service.SaveAndSet(c); err != nil {
		return err
	}
	if c.OllamaURL == "http://127.0.0.1:11435" {
		close(p.saved)
		<-p.resume
	}
	return nil
}
func TestUpdateConfigSnapshotActivationSerialized(t *testing.T) {
	credentialtest.Enable(t)
	s := routingUpdateServer(t, "open_only")
	runtime := &recordingReconfigure{ComposableResolver: s.providerSvc}
	s.providerSvc = runtime
	paused := &pausedSnapshotService{Service: s.cfgSvc, saved: make(chan struct{}), resume: make(chan struct{})}
	s.cfgSvc = paused
	a := make(chan error, 1)
	b := make(chan error, 1)
	go func() {
		_, err := s.UpdateConfig(context.Background(), &proto.UpdateConfigRequest{OllamaUrl: "http://127.0.0.1:11435"})
		a <- err
	}()
	<-paused.saved
	go func() {
		_, err := s.UpdateConfig(context.Background(), &proto.UpdateConfigRequest{OllamaUrl: "http://127.0.0.1:11436"})
		b <- err
	}()
	select {
	case err := <-b:
		close(paused.resume)
		<-a
		t.Fatalf("B completed while A publication/activation paused: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(paused.resume)
	if err := <-a; err != nil {
		t.Fatal(err)
	}
	if err := <-b; err != nil {
		t.Fatal(err)
	}
	if runtime.endpoint != "http://127.0.0.1:11436" {
		t.Fatal("runtime regressed")
	}
	if got := s.cfgSvc.Get().OllamaURL; got != "http://127.0.0.1:11436" {
		t.Fatalf("latest config lost: %s", got)
	}
}

func TestWatcherSnapshotRejectsStaleCallback(t *testing.T) {
	s := newConfigWatcherTestServer(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	next, err := config.SaveSnapshot(c, path, c.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	c = c.WithSnapshot(next)
	if err := s.SetConfigPersistence(path, c); err != nil {
		t.Fatal(err)
	}
	newer := c.Clone()
	newer.Port = "12456"
	if _, err := config.SaveSnapshot(newer, path, newer.Snapshot()); err != nil {
		t.Fatal(err)
	}
	s.configApplyMu.Lock()
	_, err = s.updateConfigLocked(t.Context(), &proto.UpdateConfigRequest{}, &c)
	s.configApplyMu.Unlock()
	if err == nil {
		t.Fatal("stale callback accepted")
	}
	if s.cfgSvc.Get().Port != c.Port {
		t.Fatal("stale callback changed config")
	}
}
