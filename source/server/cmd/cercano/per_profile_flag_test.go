package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

// The bind address keeps the legacy behavior for the default profile with the
// flag off: all interfaces at the configured port, as before shared hosts.
func TestAgentBindAddrDefaultKeepsConfiguredPort(t *testing.T) {
	t.Setenv(paths.PerProfileAgentsEnv, "")
	t.Setenv("CERCANO_PORT", "")
	cfg := config.Defaults()
	if got := agentBindAddr(paths.DefaultProfile, cfg); got != ":"+cfg.Port {
		t.Errorf("agentBindAddr(default) = %q, want all interfaces at the configured port %q", got, cfg.Port)
	}
}

// restart-after-upgrade decides from each agent's own launch environment —
// the one its replacement inherits — never from its own setting of the flag:
// an agent whose replacement would refuse its profile (a named profile's
// agent started without the flag on, as by f79bf87a, which predates the flag)
// is reported and left running before anything is stopped, and that is not a
// failure; every other agent is restarted, including a flag-on named
// profile's agent when this command runs with the flag off.
func TestUpgradeRestartDecidesFromEachAgentsLaunchEnvironment(t *testing.T) {
	for _, coordinator := range []string{"", "1"} {
		t.Run("coordinator flag "+strconv.Quote(coordinator), func(t *testing.T) {
			t.Setenv(paths.PerProfileAgentsEnv, coordinator)
			agents := []profileAgent{
				fakeProfileAgent(t, "default", "127.0.0.1:5001"),
				fakeProfileAgent(t, "flagon", "127.0.0.1:5002"),
				fakeProfileAgent(t, "preflag", "127.0.0.1:5003"),
				fakeProfileAgent(t, "off", "127.0.0.1:5004"),
			}
			envs := map[string][]string{
				"default": {"HOME=/home/u", "CERCANO_PROFILE="},
				"flagon":  {"HOME=/home/u", "CERCANO_PROFILE=flagon", paths.PerProfileAgentsEnv + "=1"},
				"preflag": {"HOME=/home/u", "CERCANO_PROFILE=preflag"},
				"off":     {"HOME=/home/u", "CERCANO_PROFILE=off", paths.PerProfileAgentsEnv + "=0"},
			}
			var log eventLog
			var out bytes.Buffer
			code := upgradeRestartCommand(nil, &out, profileDeps(t, &log, agents, restartOK, envs))
			if code != 0 {
				t.Fatalf("code=%d output=%s", code, out.String())
			}
			if got, want := log.String(), "restart:default,restart:flagon,left:preflag,left:off,modelhost"; got != want {
				t.Fatalf("events = %s, want %s", got, want)
			}
			for _, want := range []string{
				`Restarted agent for profile "default".`,
				`Restarted agent for profile "flagon".`,
				`Left the agent for profile "preflag" running: it was started without ` + paths.PerProfileAgentsEnv + `=1`,
				`Left the agent for profile "off" running: it was started without ` + paths.PerProfileAgentsEnv + `=1`,
				"2 running agents restarted",
			} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output lacks %q: %s", want, out.String())
				}
			}
			if strings.Contains(out.String(), "failed") {
				t.Errorf("a refused replacement is not a failure: %s", out.String())
			}
		})
	}
}

// replacementCheck judges the replacement's environment exactly as the
// replacement will read it: CERCANO_PROFILE (default when empty), the flag
// by strconv.ParseBool, the last of duplicate entries, and never this
// process's own environment.
func TestReplacementCheck(t *testing.T) {
	flag := paths.PerProfileAgentsEnv
	for _, tc := range []struct {
		name  string
		want  string
		env   []string
		allow bool
	}{
		{"unset profile is default", "", nil, true},
		{"empty profile is default", paths.DefaultProfile, []string{"CERCANO_PROFILE="}, true},
		{"default ignores the flag", paths.DefaultProfile, []string{"CERCANO_PROFILE=", flag + "=0"}, true},
		{"named without flag", "work", []string{"CERCANO_PROFILE=work"}, false},
		{"named with flag off", "work", []string{"CERCANO_PROFILE=work", flag + "=false"}, false},
		{"named with unparsable flag", "work", []string{"CERCANO_PROFILE=work", flag + "=yes"}, false},
		{"named with flag on", "work", []string{"CERCANO_PROFILE=work", flag + "=true"}, true},
		{"last duplicate wins (off)", "work", []string{"CERCANO_PROFILE=work", flag + "=1", flag + "=0"}, false},
		{"last duplicate wins (on)", "work", []string{"CERCANO_PROFILE=work", flag + "=0", flag + "=1"}, true},
		{"invalid profile", "", []string{"CERCANO_PROFILE=Not Valid", flag + "=1"}, false},
		{"another profile than recorded", "work", []string{"CERCANO_PROFILE=play", flag + "=1"}, false},
		{"address of a named agent", "", []string{"CERCANO_PROFILE=play", flag + "=1"}, true},
	} {
		for _, own := range []string{"", "1"} {
			t.Setenv(flag, own)
			err := replacementCheck(tc.want)(tc.env)
			if (err == nil) != tc.allow {
				t.Errorf("%s (own flag %q): err=%v, want allowed=%v", tc.name, own, err, tc.allow)
			}
		}
	}
}

// commandProfileHostUse reads each command line the way the command itself
// does: only commands that would run, connect to or start the active
// profile's agent host use it; an explicit address, a help request or an
// argument error before any host is reached does not.
func TestCommandProfileHostUse(t *testing.T) {
	enterprise := noProfileHost
	if runtime.GOOS == "darwin" {
		enterprise = dialsProfileHost
	}
	for _, tc := range []struct {
		args []string
		want profileHostUse
	}{
		{[]string{"cercano"}, servesProfileHost},
		{[]string{"cercano", "agent"}, servesProfileHost},
		{[]string{"cercano", "--server"}, servesProfileHost},
		{[]string{"cercano", "unknown-subcommand"}, servesProfileHost},
		{[]string{"cercano", "--mcp"}, mcpProfileHost},
		{[]string{"cercano", "--mcp", "--grpc-addr", "127.0.0.1:1"}, noProfileHost},
		{[]string{"cercano", "--mcp", "--grpc-addr=127.0.0.1:1"}, noProfileHost},
		{[]string{"cercano", "--version"}, noProfileHost},
		{[]string{"cercano", "version"}, noProfileHost},
		{[]string{"cercano", "--stats"}, noProfileHost},
		{[]string{"cercano", "-h"}, noProfileHost},
		{[]string{"cercano", "--no-such-flag"}, noProfileHost},
		{[]string{"cercano", "run", "hello"}, dialsProfileHost},
		{[]string{"cercano", "run", "--auto-allow", "--conv", "c1", "hello"}, dialsProfileHost},
		{[]string{"cercano", "run"}, noProfileHost},
		{[]string{"cercano", "run", "--help"}, noProfileHost},
		{[]string{"cercano", "run", "--conv"}, noProfileHost},
		{[]string{"cercano", "enterprise", "status"}, enterprise},
		{[]string{"cercano", "enterprise", "status", "--address", "127.0.0.1:1"}, noProfileHost},
		{[]string{"cercano", "enterprise", "--help"}, noProfileHost},
		{[]string{"cercano", "enterprise", "bogus"}, noProfileHost},
		{[]string{"cercano", "restart-after-upgrade"}, noProfileHost},
		{[]string{"cercano", "setup"}, noProfileHost},
		{[]string{"cercano", "stats"}, noProfileHost},
		{[]string{"cercano", "logs"}, noProfileHost},
		{[]string{"cercano", "export", "trajectory"}, noProfileHost},
		{[]string{"cercano", "reset"}, noProfileHost},
		{[]string{"cercano", "worker"}, noProfileHost},
		{[]string{"cercano", "model-host"}, noProfileHost},
	} {
		if got := commandProfileHostUse(tc.args); got != tc.want {
			t.Errorf("commandProfileHostUse(%q) = %d, want %d", tc.args, got, tc.want)
		}
	}
}

// disabledProfileHost refuses exactly a named profile with the flag off, for
// a command that uses the profile's host, with the error that names the flag.
func TestDisabledProfileHost(t *testing.T) {
	for _, tc := range []struct {
		profile, flag string
		args          []string
		refuse        bool
	}{
		{"work", "", []string{"cercano", "agent"}, true},
		{"work", "0", []string{"cercano", "agent"}, true},
		{"work", "false", []string{"cercano"}, true},
		{"work", "", []string{"cercano", "--mcp"}, true},
		{"work", "", []string{"cercano", "run", "hello"}, true},
		{"work", "", []string{"cercano", "--mcp", "--grpc-addr", "127.0.0.1:1"}, false},
		{"work", "", []string{"cercano", "stats"}, false},
		{"work", "1", []string{"cercano", "agent"}, false},
		{"work", "true", []string{"cercano", "--mcp"}, false},
		{"", "", []string{"cercano", "agent"}, false},
		{"default", "0", []string{"cercano", "--mcp"}, false},
	} {
		t.Setenv("CERCANO_PROFILE", tc.profile)
		t.Setenv(paths.PerProfileAgentsEnv, tc.flag)
		_, err := disabledProfileHost(tc.args)
		if (err != nil) != tc.refuse {
			t.Errorf("profile %q flag %q %q: err=%v, want refused=%v", tc.profile, tc.flag, tc.args, err, tc.refuse)
		}
		if err != nil && err.Error() != errProfileHostDisabled(tc.profile).Error() {
			t.Errorf("refusal %q, want %q", err, errProfileHostDisabled(tc.profile))
		}
	}
}
