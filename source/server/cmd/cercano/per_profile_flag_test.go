package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
)

// With per-profile agent hosts off (the default), only the default profile
// may run a host: acquiring any other profile's host fails at once, before
// its directory or lock exists, with an error that names the flag. The
// default profile still gets the single host.
func TestAcquireAgentHostGatedOffByDefault(t *testing.T) {
	t.Setenv(paths.PerProfileAgentsEnv, "")

	_, err := acquireAgentHost("work")
	if err == nil || !strings.Contains(err.Error(), paths.PerProfileAgentsEnv) {
		t.Fatalf("acquireAgentHost(work) err = %v, want the flag hint", err)
	}
	if entries, statErr := os.ReadDir(paths.ProfileDir("work")); statErr == nil {
		t.Errorf("profile directory was created for a gated-off profile: %v", entries)
	}

	host, err := acquireAgentHost(paths.DefaultProfile)
	if err != nil {
		t.Fatalf("acquireAgentHost(default) with the flag off: %v", err)
	}
	host.close()
}

// With the flag on, any profile may run its own host again.
func TestAcquireAgentHostAllowedWhenFlagOn(t *testing.T) {
	t.Setenv(paths.PerProfileAgentsEnv, "1")

	host, err := acquireAgentHost("work")
	if err != nil {
		t.Fatalf("acquireAgentHost(work) with the flag on: %v", err)
	}
	host.close()
	if _, err := os.Stat(filepath.Join(paths.ProfileDir("work"), agentLockName)); err != nil {
		t.Errorf("profile agent lock missing after close: %v", err)
	}
}

// The bind address keeps the legacy behavior for the default profile, so
// clients that dial the configured port find the single host.
func TestAgentBindAddrDefaultKeepsConfiguredPort(t *testing.T) {
	t.Setenv(paths.PerProfileAgentsEnv, "")
	t.Setenv("CERCANO_PORT", "")
	cfg := config.Defaults()
	if got := agentBindAddr(paths.DefaultProfile, cfg); got != "127.0.0.1:"+cfg.Port {
		t.Errorf("agentBindAddr(default) = %q, want the configured port %s", got, cfg.Port)
	}
}

// With per-profile agent hosts off, a named profile's leftover agent from a
// flag-on run is left running: restart-after-upgrade neither restarts it nor
// takes its launch lock, and says why. The default profile's agent still
// restarts, and the model host is still replaced.
func TestUpgradeRestartSkipsNamedProfilesWhenFlagOff(t *testing.T) {
	t.Setenv(paths.PerProfileAgentsEnv, "")
	agents := []profileAgent{
		fakeProfileAgent(t, "default", "127.0.0.1:5001"),
		fakeProfileAgent(t, "work", "127.0.0.1:5002"),
	}
	var log eventLog
	var out bytes.Buffer
	code := upgradeRestartCommand(nil, &out, profileDeps(t, &log, agents, restartOK))
	if code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	if got, want := log.String(), "restart:default,modelhost"; got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	if _, err := os.Stat(filepath.Join(agents[1].Dir, profileLaunchLockName)); err == nil {
		t.Error("the skipped profile's launch lock was taken")
	}
	for _, want := range []string{
		`Restarted agent for profile "default".`,
		`Left the agent for profile "work" running`,
		paths.PerProfileAgentsEnv + "=1",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q: %s", want, out.String())
		}
	}
}
