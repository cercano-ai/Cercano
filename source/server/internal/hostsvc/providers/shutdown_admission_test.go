package providers

import (
	"cercano/source/server/internal/cloudfactory"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"context"
	"errors"
	"testing"
)

func TestShutdownAdmissionQueuedRebuildZeroOpeners(t *testing.T) {
	// A nil config service is intentional: even config/credential resolution
	// must not begin after the queued rebuild obtains the serialization lock.
	p := &service{}
	p.rebuildMu.Lock()
	queued := make(chan struct{})
	done := make(chan error, 1)
	go func() { close(queued); done <- p.Rebuild() }()
	<-queued
	p.FenceRebuild()
	p.rebuildMu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestShutdownAdmissionEnteredRebuildSurvives(t *testing.T) {
	t.Setenv(secrets.DisableEnv, "0") // memory credentials and fake builder only
	prof := config.CloudProfile{Name: "fixture", Flavor: cloudfactory.FlavorMessages, Route: cloudfactory.RouteSubscription, Backend: "anthropic", Model: "fixture"}
	c := config.Defaults()
	c.ActiveCloudProfile = prof.Name
	c.CloudProfiles = []config.CloudProfile{prof}
	p := &service{cfgSvc: cfgsvc.New("", c, secrets.NewMemory()), router: &recordingRouter{}}
	entered, release := make(chan struct{}), make(chan struct{})
	p.SetCloudBuilder(func(config.CloudProfile, string, ...cloudfactory.Options) (inference.Provider, error) {
		close(entered)
		<-release
		return &labelProvider{}, nil
	})
	done := make(chan error, 1)
	go func() { done <- p.Rebuild() }()
	<-entered
	p.FenceRebuild()
	joined := make(chan struct{})
	go func() { p.WaitForRebuild(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("joined entered builder")
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	<-joined
	if p.Cloud() == nil {
		t.Fatal("entered rebuild lost publication")
	}
}
