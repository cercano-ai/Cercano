// Command cercano-cli is the standalone terminal client for Cercano.
//
// It is a thin gRPC client: it connects to the agent for its profile (chosen
// with --profile or CERCANO_PROFILE, default "default"), found through the
// profile's agent.json, or auto-launches one (as `cercano agent`) if none is
// running. Every client of a profile (this CLI, the VS Code extension, MCP)
// shares that profile's agent and conversation store.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"cercano/source/clients/cli/internal/buildversion"
	cliui "cercano/source/clients/cli/internal/ui"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
)

// version is set at build time via -ldflags "-X main.version=...".
// Falls back to "dev" for local builds.
var version = "dev"

// dialAgent connects to a profile's agent, launching it when none runs.
// Tests replace it.
var dialAgent = agentclient.DialProfile

// cliFlags is the parsed command line.
type cliFlags struct {
	resume, setup, mdtest, version bool
	profile                        string
	profileSet                     bool
}

// parseFlags defines the CLI's flags on fs and parses args.
func parseFlags(fs *flag.FlagSet, args []string) (cliFlags, error) {
	resumeShort := fs.Bool("r", false, "Open the conversation history picker on launch (alias for --resume)")
	resumeLong := fs.Bool("resume", false, "Open the conversation history picker on launch")
	setupShort := fs.Bool("s", false, "Open the setup wizard on launch (alias for --setup)")
	setupLong := fs.Bool("setup", false, "Open the setup wizard on launch")
	mdtest := fs.Bool("mdtest", false, "Launch the TUI with a markdown doc pre-loaded for render testing (optional file path as a positional arg; built-in sample if omitted)")
	showVersion := fs.Bool("version", false, "Print version and exit")
	profile := fs.String("profile", "", "Use this profile and its agent (overrides CERCANO_PROFILE)")
	if err := fs.Parse(args); err != nil {
		return cliFlags{}, err
	}
	f := cliFlags{
		resume:  *resumeShort || *resumeLong,
		setup:   *setupShort || *setupLong,
		mdtest:  *mdtest,
		version: *showVersion,
		profile: *profile,
	}
	fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "profile" {
			f.profileSet = true
		}
	})
	return f, nil
}

// selectProfile makes --profile the active profile for this process and for
// the agent it launches; it wins over CERCANO_PROFILE. Without the flag the
// environment decides.
func selectProfile(f cliFlags) error {
	if !f.profileSet {
		return nil
	}
	if err := paths.ValidateProfileName(f.profile); err != nil {
		return err
	}
	return os.Setenv("CERCANO_PROFILE", f.profile)
}

func main() {
	f, _ := parseFlags(flag.CommandLine, os.Args[1:]) // CommandLine exits on a parse error
	// The profile is settled before anything reads a path, migrates data or
	// launches an agent.
	if err := selectProfile(f); err != nil {
		fmt.Fprintln(os.Stderr, "cercano: --profile:", err)
		os.Exit(2)
	}

	if f.version {
		fmt.Printf("cercano-cli %s\n", buildversion.Display(version))
		return
	}

	// Profile files (config, debug logs) need a home directory and a valid
	// CERCANO_PROFILE; fail readably before any path is computed.
	if err := paths.CheckHome(); err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}
	if _, err := paths.Profile(); err != nil {
		fmt.Fprintln(os.Stderr, "cercano: CERCANO_PROFILE:", err)
		os.Exit(1)
	}

	// Data written before profiles existed moves into the default profile
	// before any profile file is read.
	if err := migrateProfileData(); err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}

	// Config is shared with the agent (pkg/config), which applies it; a
	// malformed file is reported here before the TUI takes the screen.
	if _, err := config.Load(config.DefaultPath()); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to load config: %v\n", err)
	}

	openHistory := f.resume
	// The wizard opens on explicit request, on first run (no config file
	// yet), or when a previous run was left unfinished (resume).
	openWizard := needsSetup(f.setup, config.DefaultPath())
	seedDoc := ""
	if f.mdtest {
		// Render-testing mode: launch the TUI with a markdown doc pre-loaded.
		// No model round-trip — the doc renders through the normal viewport path.
		seedDoc = loadMdTestDoc(flag.Arg(0))
	}
	runCLI(openHistory, openWizard, seedDoc)
}

// connectAgent connects to the active profile's agent, launching it when
// none runs, and reports progress to status.
func connectAgent(ctx context.Context, status io.Writer) (*agentclient.Client, error) {
	profile, err := paths.Profile()
	if err != nil {
		return nil, fmt.Errorf("CERCANO_PROFILE: %w", err)
	}
	fmt.Fprintf(status, "cercano: connecting to the agent for profile %q…\n", profile)
	ag, err := dialAgent(ctx, profile)
	if err != nil {
		return nil, err
	}
	if ag.AutoLaunched {
		fmt.Fprintln(status, "cercano: auto-launched agent server (log:", ag.ServerLog+")")
	}
	return ag, nil
}

// runCLI launches the cercano TUI connected to the active profile's agent.
// openHistoryOnStart opens the /history picker immediately after first paint
// (used by the -r / --resume flag).
func runCLI(openHistoryOnStart, openWizardOnStart bool, seedDoc string) {
	ag, err := connectAgent(context.Background(), os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}
	defer ag.Close()

	m := cliui.New(ag, openHistoryOnStart).WithVersion(version)
	if openWizardOnStart {
		m = m.OpenWizardOnStart()
	}
	if seedDoc != "" {
		m = m.SeedAssistantMarkdown(seedDoc)
	}
	// TEMPORARY: log the detected color profile before the program starts, so
	// we can diagnose the paste color-loss bug. Remove once diagnosed.
	cliui.LogStartupColorProfile()
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}
}
