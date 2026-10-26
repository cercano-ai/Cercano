// Command cercano-cli is the standalone terminal client for Cercano.
//
// It is a thin gRPC client: it connects to a running cercano agent server on
// localhost:<port>, or auto-launches one (as `cercano agent`) if none is
// listening. The agent server is a singleton — multiple clients (this CLI, the
// VS Code extension, etc.) share the same server and conversation store.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
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
	resumeShort := flag.Bool("r", false, "Open the conversation history picker on launch (alias for --resume)")
	resumeLong := flag.Bool("resume", false, "Open the conversation history picker on launch")
	setupShort := flag.Bool("s", false, "Open the setup wizard on launch (alias for --setup)")
	setupLong := flag.Bool("setup", false, "Open the setup wizard on launch")
	mdtest := flag.Bool("mdtest", false, "Launch the TUI with a markdown doc pre-loaded for render testing (optional file path as a positional arg; built-in sample if omitted)")
	showVersion := flag.Bool("version", false, "Print version and exit")
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
					_, err = tea.NewProgram(cliui.New(ag, false).WithVersion(version).WithDevHost(s, version)).Run()
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
	addr := "localhost:" + cfg.Port
	fmt.Fprintln(os.Stderr, "cercano: connecting to", addr+"…")
	ag, err := agentclient.Dial(context.Background(), addr)
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
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		os.Exit(1)
	}
}
