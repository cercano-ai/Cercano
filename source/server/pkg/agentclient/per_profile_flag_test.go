package agentclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/pkg/paths"
)

// failingLaunch records launch attempts and always fails, so a test can
// prove whether a dial tried to start an agent.
type failingLaunch struct {
	called  bool
	profile string
}

func (f *failingLaunch) install(t *testing.T) {
	t.Helper()
	prev := launchFunc
	launchFunc = func(profile, logPath string) (<-chan struct{}, error) {
		f.called = true
		f.profile = profile
		return nil, errors.New("test: no launch")
	}
	t.Cleanup(func() { launchFunc = prev })
}

// With per-profile agent hosts off (the default), only the default profile
// is routable: every other profile fails at once, before any lock is taken
// or agent launched, with an error that names the flag.
func TestDialProfileGatedOffByDefault(t *testing.T) {
	t.Setenv(paths.PerProfileAgentsEnv, "")
	var launch failingLaunch
	launch.install(t)

	_, err := DialProfile(context.Background(), "work")
	if !errors.Is(err, ErrPerProfileAgentsDisabled) {
		t.Fatalf("DialProfile(work) err = %v, want ErrPerProfileAgentsDisabled", err)
	}
	if !strings.Contains(err.Error(), paths.PerProfileAgentsEnv) {
		t.Errorf("DialProfile(work) err = %q, want it to name %s", err, paths.PerProfileAgentsEnv)
	}
	if launch.called {
		t.Error("an agent was launched for a gated-off profile")
	}
	if _, statErr := os.Stat(filepath.Join(paths.ProfileDir("work"), "launch.lock")); statErr == nil {
		t.Error("a launch lock was taken for a gated-off profile")
	}

	_, err = DialProfileExisting(context.Background(), "work")
	if !errors.Is(err, ErrPerProfileAgentsDisabled) {
		t.Fatalf("DialProfileExisting(work) err = %v, want ErrPerProfileAgentsDisabled", err)
	}
	if launch.called {
		t.Error("DialProfileExisting launched an agent for a gated-off profile")
	}
}

// The default profile keeps the single host when the flag is off: its dial
// still auto-launches, exactly as before the per-profile feature.
func TestDialProfileDefaultAllowedWhenOff(t *testing.T) {
	t.Setenv(paths.PerProfileAgentsEnv, "")
	var launch failingLaunch
	launch.install(t)

	_, err := DialProfile(context.Background(), paths.DefaultProfile)
	if err == nil || !strings.Contains(err.Error(), "could not auto-launch") {
		t.Fatalf("DialProfile(default) err = %v, want the stub launch failure", err)
	}
	if !launch.called || launch.profile != paths.DefaultProfile {
		t.Errorf("launch = %+v, want one attempt for the default profile", launch)
	}
}

// With the flag on, non-default profiles route through the per-profile
// host machinery again.
func TestDialProfileAllowedWhenFlagOn(t *testing.T) {
	t.Setenv(paths.PerProfileAgentsEnv, "1")
	var launch failingLaunch
	launch.install(t)

	_, err := DialProfile(context.Background(), "work")
	if err == nil || !strings.Contains(err.Error(), "could not auto-launch") {
		t.Fatalf("DialProfile(work) err = %v, want the stub launch failure", err)
	}
	if !launch.called || launch.profile != "work" {
		t.Errorf("launch = %+v, want one attempt for profile work", launch)
	}
}
