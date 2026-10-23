# Cercano CLI

The terminal client is implemented in `source/clients/cli`, a separate Go module.
It is a Bubble Tea v2 TUI and a gRPC client of the shared Cercano agent. There
are **two binaries**: `cercano-cli` (terminal UI) and `cercano` (agent/MCP host).
This guide describes the current source interface, not the original roadmap.

## Build and start

The CLI module declares Go **1.26.0**. From the repository root, build both
binaries into the same directory:

```sh
(cd source/server && go build -o bin/cercano ./cmd/cercano/)
(cd source/clients/cli && go build -o ../../server/bin/cercano-cli .)
./source/server/bin/cercano-cli
```

The CLI reads `~/.config/cercano/config.yaml`, connects to `localhost:<port>`
(default `50052`), and auto-launches `cercano agent` if needed. It locates the
server next to its own executable, then on `PATH`. The server outlives the UI;
multiple clients share it. This is not an embedded server or a headless prompt
command. Server commands such as `agent` and `--mcp` belong to `cercano`, not
`cercano-cli`.

Supported startup options:

| Option | Effect |
|---|---|
| `--resume`, `-r` | Open the conversation history picker, not automatically the latest conversation. |
| `--setup`, `-s` | Open setup explicitly. |
| `--mdtest [file]` | Seed a markdown document into the TUI; built-in sample if omitted. Still connects to the agent, but rendering the sample needs no inference. |
| `--version` | Print client version and exit. |
| `--help` | Show Go flag help. |

Setup also opens when the server config file is missing or saved wizard state
indicates an unfinished setup. `/setup` reopens it without resetting settings.
There are no client `--yolo` or `--bypass-permissions` flags.

## Commands inside the TUI

Use `/help` for the registered commands. Parsing is local, whitespace-split and
offers prefix completion/suggestions (dispatch requires an exact command or alias); these are not shell subcommands. Settings and
server actions use RPCs; normal chat, `/auto`, and context summarization can
involve inference.

| Command | Current behavior |
|---|---|
| `/help`, `/quit` (`/exit`) | List commands or exit the client. |
| `/clear` | Clear displayed messages and start a new conversation ID; does not delete the old stored conversation. |
| `/history`, `/resume [id]` | Open history, or resume an exact conversation ID. |
| `/rename <title>` or `/rename <id> <title>` | Rename the current or specified conversation. |
| `/search [text]` | Search readable messages in this conversation; also available with Ctrl+F. |
| `/export trajectory [path]`, `/traj [path]` | Open trajectory-bundle export. |
| `/config`, `/s` (`/settings`) | Open configuration. `/config show` prints current config. |
| `/config <key> <value>` | Update a supported config key; see below. |
| `/cloud [list]`, `/cloud use <name>`, `/cloud key <name> <api-key>` | List saved profiles, select the active profile, or store its key. Prefer the masked Cloud editor for credentials. |
| `/model [status\|models <profile>\|set <profile> <exact-model-id>\|clear]` | Inspect, pin or clear the session's main-chat model. |
| `/escalate`, `/ultrathink` | Pin this session to the configured Premium model; send the message after success. No arguments. |
| `/locus [cloud_only\|cloud_primary\|local_primary\|local_only]` | Inspect or change locus policy. |
| `/m`, `/runtime` | Open Local Models management or Runtime configuration. |
| `/mcp [list]` | Open the MCP configuration tab. |
| `/mcp add <name> <command> [args…]`, `/mcp remove <name>`, `/mcp restart <name>` | Manage hosted MCP servers. |
| `/tools`, `/tool <name> <json-args>` | List the live registry or directly invoke a named tool. Permission enforcement still applies. |
| `/strict`, `/permissive`, `/bypass`, `/mode <strict\|permissive\|bypass>` | Set agent permission mode. `/bypass` is a mode selection, not an on/off/status command. |
| `/plan [off\|profile]` | Enter planning mode (bare command), return to default, or select a profile. |
| `/auto [goal]` | Submit a normal prompt asking for an autonomous run brief and approval; does not itself enable autonomous execution. |
| `/context` | Show `.cercano/context.md` under the effective working directory. |
| `/c` | Open the active conversation's context viewer. |
| `/compact` | Incrementally compact the backlog. |
| `/context-regen` | Rebuild derived context from raw turns, rerunning compaction. |
| `/clear-compacted-context` | Drop derived summaries and rehydrate raw history without summarizing. |
| `/elide-context` | Stub existing tool-result bodies in the send-view, not stored raw turns; resets on agent restart. |
| `/theme`, `/color <name\|#RRGGBB>` | Open theme settings or change prompt-border color. |
| `/setup` | Open/resume setup. |
| `/d [repo-path]` (`/dev`) | Enter Cercano self-development mode. |
| `/restart-agent [reason]` (`/bounce`) | Request agent restart and reconnect. |

The UI queues confirmations for known write/destructive direct tool calls;
the unary invocation RPC can still reject operations requiring streaming approval.
Use normal chat for interactive model-driven tool approvals. Do not infer tool
names from old plans: `/tools` is authoritative. There are no registered `/font`,
`/diff`, `/undo`, or `/project` commands.

## Configuration and accounts

The unified configuration tabs are **General, Cloud, Routing, Runtime, Local
Models, MCP, UI, Token Metrics, Context**. Cloud manages account drafts and
credentials; Routing manages task destinations. Multiple saved accounts of the
same provider and ordered Primary backups are supported. See [Cloud routing](../../cloud-routing.md)
and the [model catalog picker](model-catalog-picker.md). A session model pin
is separate from global routing and has no fallback; see [session models](session-model.md).

`/config` accepts `local-runtime`, `local-model`, `ollama-url`, `cloud-provider`,
`cloud-api-key`, `cloud-base-url`, `compaction-enabled`,
`tool-elision-only`, `elide-tool-results`, `lossy-tool-elision`,
`raw-retention-days`, `compacted-retention-days`, and `keep-forever`
(underscore aliases also exist). Sparse open-model overrides use
`/config models.<runtime>.<tier> <model-id>`; `-` clears an override. Runtime/tier
validation is server-side; for example, `models.llama_server.fast_light_text`.
These keys are not a complete YAML schema. The legacy `cloud-model`
(`cloud_model`) key is still parsed by the CLI, but non-empty updates are rejected
by the server as retired. Edit profile quality choices in the **Cloud** tab and
task destinations in **Routing** instead; see [profile quality choices](../../cloud-routing.md#quality-and-image-choices-belong-to-profiles).
Prefer these tabs for account-aware configuration rather than legacy single-cloud fields.

CLI-only theme selection is stored in `ui.yaml`, with custom themes in
`themes/<name>.yaml`. These use `$XDG_CONFIG_HOME/cercano` or
`~/.config/cercano`, with `CERCANO_UI_CONFIG` and `CERCANO_THEMES_DIR` overrides.
The server's `config.yaml` default is independently resolved under the home
directory. The active-theme fallback is `cr4k3r_j4x`, not the old `cracker` name.

## Implemented terminal features

- Streaming markdown, tables and diff rendering, cached transcript layout,
  virtual scrolling, mouse selection and draggable scrollbars.
- Multiline prompt, pasted text, prompt history, queued follow-up turns, and
  atomic image attachment chips. Dropped image files support PNG, JPEG, GIF and
  WebP up to 20 MiB each. Clipboard image extraction is macOS-only (pngpaste or
  osascript); other platforms can use file paths. Model/provider vision support
  is still required for useful image interpretation.
- Conversation history/resume and recap display, in-conversation search,
  context inspection/compaction, trajectory export, and token metrics.
- Shared chat views, conversation/task tabs, delegated-task subscriptions and
  progress panes. See [independent tasks](../../agent/independent-tasks.md).
- Hosted MCP configuration, runtime/model management, cloud account/catalog
  editing, setup wizard, themes, permission modes and autonomous-run approval.

These are implementation claims, not a claim that every terminal, model,
provider account or live MCP server has been manually tested in this audit.

## Implementation map and historical designs

Paths below are relative to `source/clients/cli` unless noted. Feature design
and plan documents in this directory are **historical records**, including their
checkboxes, examples and test reports. They are not installation instructions.
The [original roadmap](original-plan.md) is preserved rather than rewritten.

| Historical feature group | Current implementation evidence |
|---|---|
| buffer-scrollbar | `internal/ui/scrollbar.go`, `chat_virtual_scroll.go`, `chat_view.go` |
| chat-view (steps 1–4) | `internal/ui/chat_view.go`, `chat_events.go`, `main_agent_driver.go`, `context_manager_driver.go` |
| dev-mode | `internal/slash/dev.go`, `internal/ui/model.go` (`applyDevMode`) |
| history-view / living-recap | `internal/ui/history_view.go`; server conversation and recap services |
| image-input / textarea | `internal/ui/image_paste.go`, `clipboard_image_*.go`, `prompt_input.go`, `prompt_image.go` |
| llm-provider-wireup / native-tool-calling | server `internal/inference`, `internal/runner`, `internal/capabilities`; CLI `main_agent_driver.go` |
| mcp-host | `internal/slash/mcp.go`, configuration MCP tab; server MCP host RPCs |
| model-catalog-online | `internal/ui/cloud_catalog_picker.go`, `cloud_provider_catalog.go`, `internal/overlay/catalog.go` |
| settings / theme-system | `internal/ui/config_surface.go`, `config_tabs.go`, `settings_page.go`, `internal/theme`, `internal/uiconfig` |
| streaming-markdown | `internal/render/glamour.go`, `mdstream.go`, `table.go`, `diff.go` |
| tui-charm-v2 | `go.mod` (`charm.land/*` v2), `main.go` |

For audit coverage, evidence and verification limits, see [CLI audit](../../audit-cli.md).
