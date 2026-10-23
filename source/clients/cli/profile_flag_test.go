package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/profilemigrate"
)

func TestProfileFlagOverridesEnv(t *testing.T) {
	t.Setenv("CERCANO_PROFILE", "home")
	fs := flag.NewFlagSet("cercano-cli", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	f, err := parseFlags(fs, []string{"--profile", "work", "-r"})
	if err != nil {
		t.Fatal(err)
	}
	if !f.resume {
		t.Error("-r was not parsed alongside --profile")
	}
	if err := selectProfile(f); err != nil {
		t.Fatalf("selectProfile: %v", err)
	}
	if got, err := paths.Profile(); err != nil || got != "work" {
		t.Fatalf("paths.Profile() = %q, %v; want work", got, err)
	}
	// Agents this process launches inherit the choice.
	if got := os.Getenv("CERCANO_PROFILE"); got != "work" {
		t.Errorf("CERCANO_PROFILE = %q, want work", got)
	}
}

func TestNoProfileFlagKeepsEnv(t *testing.T) {
	t.Setenv("CERCANO_PROFILE", "home")
	fs := flag.NewFlagSet("cercano-cli", flag.ContinueOnError)
	f, err := parseFlags(fs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := selectProfile(f); err != nil {
		t.Fatal(err)
	}
	if got, _ := paths.Profile(); got != "home" {
		t.Fatalf("paths.Profile() = %q, want home", got)
	}
}

// mainArgsEnv makes the re-executed test binary run main with the arguments
// after "--".
const mainArgsEnv = "CERCANO_CLI_TEST_RUN_MAIN"

func runMainInChild() {
	for i, a := range os.Args {
		if a == "--" {
			os.Args = append([]string{"cercano-cli"}, os.Args[i+1:]...)
			break
		}
	}
	dialAgent = func(context.Context, string) (*agentclient.Client, error) {
		fmt.Fprintln(os.Stderr, "TEST: agent dialed")
		os.Exit(99)
		return nil, nil
	}
	migrationOptions = func() profilemigrate.Options { panic("TEST: migration ran") }
	main()
	os.Exit(0)
}

func TestInvalidProfileFlagExitsWithMessage(t *testing.T) {
	if os.Getenv(mainArgsEnv) == "1" {
		runMainInChild()
	}
	home := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestInvalidProfileFlagExitsWithMessage$", "--", "--profile", "Bad")
	cmd.Env = append(childEnv(), mainArgsEnv+"=1", "HOME="+home, "USERPROFILE="+home, "CERCANO_PROFILE=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("exit = %v, want status 2; stderr:\n%s", err, stderr.String())
	}
	want := paths.ValidateProfileName("Bad").Error()
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
	}
	if strings.Contains(stderr.String(), "TEST:") {
		t.Errorf("an agent was dialed or data migrated: %s", stderr.String())
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Errorf("home was written to: %v", entries)
	}
}

// childEnv is the test environment without variables the child sets itself.
func childEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(key) {
		case "HOME", "USERPROFILE", "CERCANO_PROFILE", mainArgsEnv:
			continue
		}
		out = append(out, kv)
	}
	return out
}

func TestCLIConnectsToResolvedProfile(t *testing.T) {
	t.Setenv("CERCANO_PROFILE", "work")
	var dialed string
	prev := dialAgent
	dialAgent = func(_ context.Context, profile string) (*agentclient.Client, error) {
		dialed = profile
		return nil, errors.New("fake dial")
	}
	t.Cleanup(func() { dialAgent = prev })

	if _, err := connectAgent(context.Background(), io.Discard); err == nil || !strings.Contains(err.Error(), "fake dial") {
		t.Fatalf("connectAgent err = %v, want the dial error", err)
	}
	if dialed != "work" {
		t.Fatalf("dialed profile %q, want work", dialed)
	}
}
