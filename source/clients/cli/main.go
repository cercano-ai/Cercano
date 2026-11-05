// Command cercano-cli is the standalone terminal client for Cercano.
//
// It is a thin gRPC client: it connects to a running cercano agent server on
// localhost:<port>, or auto-launches one (as `cercano agent`) if none is
// listening. An explicit --address selects connect-only mode, even on loopback.
// The agent server is a singleton — multiple clients (this CLI, the
// VS Code extension, etc.) share the same server and conversation store.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"cercano/source/clients/cli/internal/buildversion"
	cliui "cercano/source/clients/cli/internal/ui"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
)

// version is set at build time via -ldflags "-X main.version=...".
// Falls back to "dev" for local builds.
var version = "dev"

func main() {
	address := flag.String("address", "", "Connect only to an existing agent at HOST:PORT (including loopback tunnels); never launch a local agent")
	resumeShort := flag.Bool("r", false, "Open the conversation history picker on launch (alias for --resume)")
	resumeLong := flag.Bool("resume", false, "Open the conversation history picker on launch")
	setupShort := flag.Bool("s", false, "Open the setup wizard on launch (alias for --setup)")
	setupLong := flag.Bool("setup", false, "Open the setup wizard on launch")
	mdtest := flag.Bool("mdtest", false, "Launch the TUI with a markdown doc pre-loaded for render testing (optional file path as a positional arg; built-in sample if omitted)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	session := flag.String("session", "", "Reconnect using an exact persisted session selection; never use current-target")
	installedState := flag.String("installed-state", "", "Select this installed agent for a new session in existing prepared private state (no migration)")
	installedRuntime := flag.String("installed-runtime-root", "", "Private runtime root for --installed-state")
	devHost := flag.String("dev-host", "", "Explicit authenticated dev-host selection JSON; never discover or launch")
	devAction := flag.String("dev-host-action", "", "Management only: list, acquire, release, retire, transfer, claim (no attachment)")
	devDestination := flag.String("dev-host-destination", "", "Exact destination selection JSON")
	devHandover := flag.String("dev-host-handover", "", "Unique handover ID (reuse only for explicit claim retry)")
	devReceipt := flag.String("dev-host-receipt", "", "Prepare receipt for explicit claim retry")
	flag.Parse()

	if *showVersion {
		fmt.Printf("cercano-cli %s\n", buildversion.Display(version))
		return
	}

	// Set only by the source development launcher. Even an empty override or
	// a selector hidden behind a positional must never reach legacy auto-launch.
	// Packaged CLI invocations use installed admission below, never legacy fallback.
	if os.Getenv("CERCANO_LAUNCHED_DEVELOPMENT") == "1" && *devHost == "" {
		fmt.Fprintln(os.Stderr, "cercano: development launcher requires nonempty --dev-host; legacy fallback refused")
		os.Exit(78)
	}

	addressSet, devSelectorSet, installedSelectorSet, sessionSet := false, false, false, false
	present := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		present[f.Name] = true
		if strings.HasPrefix(f.Name, "dev-host") {
			devSelectorSet = true
		}
		if f.Name == "session" {
			sessionSet = true
		}
		if f.Name == "installed-state" || f.Name == "installed-runtime-root" {
			installedSelectorSet = true
		}
		if f.Name == "address" {
			addressSet = true
		}
	})
	if addressSet && devSelectorSet {
		fmt.Fprintln(os.Stderr, "cercano: --address is incompatible with --dev-host and dev-host management flags")
		os.Exit(1)
	}
	if err := validateDevHostArgs(present, *devHost, *devAction, *devDestination, *devHandover, *devReceipt); err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}
	// Ordinary installed entry uses the same adapter as explicit selection.
	// Explicit remote and source-development lanes keep their own routing.
	if !addressSet && !devSelectorSet && !sessionSet && !installedSelectorSet {
		var err error
		installedSelectorSet, err = discoverInstalledEntry()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cercano: installed activation deferred:", err)
			os.Exit(1)
		}
	}
	if sessionSet || installedSelectorSet {
		if addressSet || devSelectorSet || (sessionSet && installedSelectorSet) || (sessionSet && *session == "") {
			fmt.Fprintln(os.Stderr, "cercano: session selection is incompatible with address, dev-host or new-session selection")
			os.Exit(1)
		}
		if *session != "" {
			*devHost = *session
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			path, err := selectInstalledSession(ctx, *installedState, *installedRuntime)
			cancel()
			if err != nil {
				fmt.Fprintln(os.Stderr, "cercano: installed activation deferred:", err)
				os.Exit(1)
			}
			*devHost = path
			fmt.Fprintln(os.Stderr, "cercano: session selection:", path, "(reconnect with --session)")
		}
	}
	if addressSet {
		if err := validateRemoteAddress(*address); err != nil {
			fmt.Fprintln(os.Stderr, "cercano:", err)
			os.Exit(1)
		}
		seedDoc := ""
		if *mdtest {
			seedDoc = loadMdTestDoc(flag.Arg(0))
		}
		// Missing local agent config says nothing about the selected server's setup.
		runCLIAddress(*address, true, *resumeShort || *resumeLong, *setupShort || *setupLong, seedDoc)
		return
	}
	if *devHost != "" {
		s, err := cliui.ReadDevHostSelection(*devHost)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if *devAction != "" {
				response, selection, e := cliui.ControlDevHost(ctx, s, *devAction, *devDestination, *devHandover, *devReceipt)
				_ = json.NewEncoder(os.Stdout).Encode(struct {
					Response  any
					Selection any
				}{response, selection})
				err = e
				if err == nil && response != nil && response.Pending {
					err = fmt.Errorf("handover pending; preserve receipt and retry explicit claim")
				}
			} else {
				var ag *agentclient.Client
				ag, err = s.Dial(ctx)
				if err == nil {
					defer ag.Close()
					var final tea.Model
					m := cliui.New(ag, *resumeShort || *resumeLong).WithVersion(version).WithDevHost(s, version)
					// Pinned startup uses the same explicit UI options, without
					// loading global config or taking the legacy auto-launch path.
					if !s.Fixture && needsPinnedSetup(*setupShort || *setupLong, s.ConfigAuthority) {
						m = m.OpenWizardOnStart()
					}
					if *mdtest {
						m = m.SeedAssistantMarkdown(loadMdTestDoc(flag.Arg(0)))
					}
					final, err = tea.NewProgram(m).Run()
					if err == nil {
						err = closeIntentionalSession(final)
					}
				}
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "cercano:", err)
			os.Exit(1)
		}
		return
	}
	if *devAction != "" {
		fmt.Fprintln(os.Stderr, "--dev-host-action requires --dev-host")
		os.Exit(1)
	}
	// Config is shared with the server (pkg/config); the CLI needs the port to
	// dial the agent.
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to load config: %v (using defaults)\n", err)
		cfg = config.Defaults()
	}

	openHistory := *resumeShort || *resumeLong
	// The wizard opens on explicit request, on first run (no config file
	// yet), or when a previous run was left unfinished (resume).
	openWizard := needsSetup(*setupShort || *setupLong, config.DefaultPath())
	seedDoc := ""
	if *mdtest {
		// Render-testing mode: launch the TUI with a markdown doc pre-loaded.
		// No model round-trip — the doc renders through the normal viewport path.
		seedDoc = loadMdTestDoc(flag.Arg(0))
	}
	runCLI(cfg, openHistory, openWizard, seedDoc)
}

// runCLI launches the cercano TUI. Connects to a running agent on
// localhost:<cfg.Port>, or auto-launches one (as `cercano agent`) on miss.
// openHistoryOnStart opens the /history picker immediately after first paint
// (used by the -r / --resume flag).
func runCLI(cfg config.Config, openHistoryOnStart, openWizardOnStart bool, seedDoc string) {
	runCLIAddress("localhost:"+cfg.Port, false, openHistoryOnStart, openWizardOnStart, seedDoc)
}

func runCLIAddress(addr string, connectOnly, openHistoryOnStart, openWizardOnStart bool, seedDoc string) {
	fmt.Fprintln(os.Stderr, "cercano: connecting to", addr+"…")
	ag, err := dialCLI(context.Background(), addr, connectOnly)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}
	defer ag.Close()
	if ag.AutoLaunched {
		fmt.Fprintln(os.Stderr, "cercano: auto-launched agent server (log:", ag.ServerLog+")")
	}

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
	final, err := p.Run()
	if err == nil {
		err = closeIntentionalSession(final)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}
}

func closeIntentionalSession(final tea.Model) error {
	m, ok := final.(cliui.Model)
	if !ok || !m.IntentionalExit() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return m.CloseSession(ctx)
}

// Address selection is explicit, not inferred from whether the host is loopback.
func validateRemoteAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return fmt.Errorf("--address requires HOST:PORT (IPv6: [HOST]:PORT)")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("--address requires a numeric port between 1 and 65535")
	}
	return nil
}

func dialCLI(ctx context.Context, addr string, connectOnly bool) (*agentclient.Client, error) {
	if connectOnly {
		c, err := agentclient.DialExisting(ctx, addr)
		if err != nil {
			return nil, fmt.Errorf("connect-only agent at %s unavailable (no local launch): %w", addr, err)
		}
		return c, nil
	}
	return agentclient.Dial(ctx, addr)
}
