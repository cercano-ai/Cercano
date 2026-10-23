package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
)

// resolveHeadlessConvID returns the conversation id for a headless run: the
// explicit --conv value verbatim, or a fresh random id when none was given.
// A random id (not the old headless-<pid>) is essential — pids recycle, and a
// recycled pid silently continues a stranger's conversation, which the
// cross-session incident showed corrupts that conversation's upstream Meridian
// lineage. Explicit continuation still works via --conv.
func resolveHeadlessConvID(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return conversation.NewID()
}

// headlessArgs is the parsed command line of `cercano run`.
type headlessArgs struct {
	autoAllow   bool
	convID      string
	positional  []string
	help        bool // -h or --help, before any missing --conv value
	missingConv bool // --conv was the last argument
}

// parseHeadlessArgs parses `cercano run` arguments. It is shared with the
// startup check so both agree on what is a help request.
func parseHeadlessArgs(args []string) headlessArgs {
	var a headlessArgs
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--auto-allow":
			a.autoAllow = true
		case "--conv":
			if i+1 < len(args) {
				a.convID = args[i+1]
				i++
			} else {
				a.missingConv = true
				return a
			}
		case "-h", "--help":
			a.help = true
			return a
		default:
			a.positional = append(a.positional, args[i])
		}
	}
	return a
}

const headlessUsage = `usage: cercano run [--auto-allow] [--conv ID] "prompt"

Sends a single prompt to the active profile's cercano agent (CERCANO_PROFILE,
default "default") and streams the response. Auto-launches the profile's
agent if one isn't already running.

Flags:
  --auto-allow   Approve W/X-tier tool confirmations automatically.
                 Default is to deny and exit 3.
  --conv ID      Continue a specific conversation. Default mints a new id.

Output:
  stdout  assistant text deltas + final response
  stderr  tool-call diagnostics, progress, errors`

// runHeadless executes a single prompt against the cercano agent and streams
// the response to stdout (assistant text) and stderr (diagnostics). Used for
// scripting and CI — no TTY required.
func runHeadless(args []string) {
	os.Exit(runHeadlessCommand(args))
}

// runHeadlessCommand is runHeadless returning the exit code.
func runHeadlessCommand(args []string) int {
	parsed := parseHeadlessArgs(args)
	if parsed.help {
		fmt.Fprintln(os.Stderr, headlessUsage)
		return 0
	}
	if parsed.missingConv {
		fmt.Fprintln(os.Stderr, "cercano run: --conv requires an ID")
		return 2
	}
	autoAllow, convID, positional := parsed.autoAllow, parsed.convID, parsed.positional

	prompt := strings.Join(positional, " ")
	if prompt == "" {
		fmt.Fprintln(os.Stderr, `usage: cercano run [--auto-allow] [--conv ID] "prompt"`)
		return 2
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\n[interrupt] cancelling…")
		cancel()
	}()

	client, err := dialActiveProfile(ctx, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		return 1
	}
	defer client.Close()
	// Count as the agent's client for the whole run (released before the
	// client closes), so an agent this run auto-launched exits after it.
	defer holdAgent(ctx, client.SubscribeEvents)()

	convID = resolveHeadlessConvID(convID)

	workDir, _ := os.Getwd()

	events, err := client.StreamChat(ctx, convID, prompt, workDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cercano:", err)
		return 1
	}

	exitCode := 0
	sawText := false
	for msg := range events {
		switch msg.Type {
		case agentclient.TypeToken:
			fmt.Print(msg.Token)
			sawText = true
		case agentclient.TypeProgress:
			fmt.Fprintf(os.Stderr, "[progress] %s\n", msg.Note)
		case agentclient.TypeToolUseStart:
			fmt.Fprintf(os.Stderr, "\n[tool] %s\n", msg.ToolName)
		case agentclient.TypeToolUseStop:
			if msg.ArgsSummary != "" {
				fmt.Fprintf(os.Stderr, "       args: %s\n", msg.ArgsSummary)
			}
		case agentclient.TypeToolExecStart:
			// already noted via TypeToolUseStart
		case agentclient.TypeToolExecComplete:
			status := "OK"
			if msg.IsError {
				status = "ERR"
			}
			fmt.Fprintf(os.Stderr, "       [%s] %s\n", status, msg.Summary)
		case agentclient.TypePermissionRequired:
			if autoAllow {
				if err := client.AllowToolCall(ctx, convID, msg.ToolUseID); err != nil {
					fmt.Fprintf(os.Stderr, "[allow error] %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "[allowed via --auto-allow] %s (%s-tier)\n", msg.ToolName, msg.Tier)
				}
			} else {
				if err := client.DenyToolCall(ctx, convID, msg.ToolUseID); err != nil {
					fmt.Fprintf(os.Stderr, "[deny error] %v\n", err)
				}
				fmt.Fprintf(os.Stderr, "[denied] %s-tier tool %s (use --auto-allow to permit)\n", msg.Tier, msg.ToolName)
				exitCode = 3
			}
		case agentclient.TypeDone:
			// Token deltas already streamed the text; only print Final if no
			// deltas were emitted (some providers/local-only paths skip them).
			if !sawText && msg.Final != "" {
				fmt.Print(msg.Final)
				sawText = true
			}
			if msg.Notice != "" {
				fmt.Fprintf(os.Stderr, "\n[notice] %s\n", msg.Notice)
			}
		case agentclient.TypeError:
			fmt.Fprintln(os.Stderr, "\n[error]", msg.Err)
			exitCode = 1
		}
	}
	if sawText {
		fmt.Println()
	}
	return finalizeHeadlessExit(exitCode, sawText, os.Stderr)
}

func finalizeHeadlessExit(exitCode int, sawText bool, stderr io.Writer) int {
	if exitCode != 0 || sawText {
		return exitCode
	}
	fmt.Fprintln(stderr, "cercano: turn completed with no assistant output")
	return 1
}

// compactJSON returns a single-line form of a JSON string. Unused but kept
// available for future arg-pretty-printing.
func compactJSON(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
