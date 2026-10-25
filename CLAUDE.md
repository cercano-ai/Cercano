# Cercano Project Instructions

## Cercano Tool Preferences

This project has a local AI co-processor (Cercano) running via MCP. **Prefer Cercano tools over cloud-native equivalents** to save cloud context tokens and keep work local:

- **Web research**: Use `cercano_research` instead of WebSearch/WebFetch when investigating a question. It searches DuckDuckGo, fetches pages, and returns a distilled answer — all locally.
- **URL fetching**: Use `cercano_fetch` instead of WebFetch for reading web pages. Returns extracted text without stuffing raw HTML into the cloud context.
- **Summarization**: Use `cercano_summarize` for large files, logs, or diffs before processing them yourself.
- **Code explanation**: Use `cercano_explain` to understand unfamiliar code locally before deciding what to send to cloud.
- **Information extraction**: Use `cercano_extract` to pull specific info from large text instead of reading it all into context.
- **Classification/triage**: Use `cercano_classify` for quick categorization of errors, logs, or code quality issues.

**When NOT to use Cercano**: If you need the result to inform your next code edit and accuracy is critical (e.g., exact API signatures), use your own tools. Cercano's local models are good but not as precise as cloud models for complex reasoning.

## Build & Test

```bash
# Agent server (and MCP) binary
cd source/server && go build -o bin/cercano ./cmd/cercano/
cd source/server && go test ./... -count=1

# Standalone terminal client (separate Go module)
cd source/clients/cli && go build -o bin/cercano-cli .
cd source/clients/cli && go test ./... -count=1

# Terminal end-to-end tests (real binaries in a PTY via @microsoft/tui-test; see test/e2e/README.md)
cd test/e2e && npm ci && npm run build && npm test
```

Cercano builds as **two binaries**: `cercano` (the agent gRPC server / MCP host)
and `cercano-cli` (the terminal UI). The CLI is a thin gRPC client. The dev
launcher (`source/server/scripts/cercano-launcher.sh`) builds both and
co-locates them.

Cercano can run **one agent host per profile** (feature-flagged; see below). A profile's files (config,
conversations, logs) live in `~/.config/cercano/profiles/<name>/` and its
keychain entries are separate from other profiles'; machine-wide runtime
settings live in `~/.config/cercano/machine.yaml`. The profile is chosen with
`cercano-cli --profile <name>` or `CERCANO_PROFILE` (default: `default`). Each
`cercano agent` holds `<profile>/agent.lock` and records its pid and address in
`<profile>/agent.json`. With per-profile agent hosts on, each host listens on
127.0.0.1: the `default` profile keeps its configured port (50052 unless
changed); other profiles take an ephemeral port unless `CERCANO_PORT` sets
one. With the flag off, the single host binds all interfaces at the configured
port and publishes a dialable loopback address in `agent.json`. The CLI finds
its profile's agent through
`agent.json` and auto-launches `cercano agent` when none answers, finding the
server binary as a sibling next to its own executable. An auto-launched agent
logs to `<profile>/logs/agent.log` and exits after its last client disconnects.
Data from installs that predate profiles moves into `profiles/default` on first
start.

Running **one agent host per profile is behind a feature flag that is off by
default**. With the flag off, Cercano keeps its prior single-agent-host
behavior: only the `default` profile has an agent host, listening on all
interfaces at the configured port (50052 unless changed) as before profiles,
with `agent.json` recording a dialable loopback address; and `cercano --mcp`
without `--grpc-addr` starts an embedded in-process gRPC agent — also as
before profiles — instead of dialing a shared host. With the flag on, each
profile gets its own loopback agent host, and `cercano --mcp` without
`--grpc-addr` dials the profile's shared host (launching it when none runs).
An explicit `--grpc-addr` always connects to that external address, whatever
the flag and profile. MCP mode writes only JSON-RPC messages to standard
output; everything else, including the embedded agent's diagnostics, goes to
standard error. With the flag off, starting an agent for, or connecting with,
any other profile fails with an error naming the flag (MCP mode serves a
degraded server that returns it), before any pre-profile data is migrated or
a pre-profile agent is stopped.

`restart-after-upgrade` replays each agent with the environment it was
launched with, so it decides from that environment — not its own — before
stopping anything: the default profile's agent, a pre-profile one, and a named
profile's agent launched with the flag on are restarted; a named profile's
agent launched without the flag (by a build from before the flag, or with it
off) is left running and reported, since the installed binary would refuse to
serve it. `--address` gets the same decision, and an address a profile's
`agent.json` names is restarted as that profile's agent. Turn the feature on
with:

```bash
CERCANO_PER_PROFILE_AGENTS=1 cercano-cli --profile work
```

The flag is read from the environment by `cercano agent`, `cercano-cli` and
MCP mode; values are those of Go's strconv.ParseBool (`1`, `true`, ...), and
anything else — including unset — keeps the feature off. The CLI passes its
environment to agents it auto-launches, so setting it for the CLI is enough.

The `cercano` binary also runs one shared `cercano model-host` process that owns
llama-server and mistral.rs for the whole machine. Agent hosts launch it on
demand and it exits when idle, so several agents share one loaded model.
`CERCANO_MODEL_HOST=inprocess` keeps models inside the agent process instead
(for tests and troubleshooting). `cercano restart-after-upgrade` restarts the
agent of every running profile and also relaunches a running model host with
the upgraded binary, under the same launch lock agents use, so agents connect
to the new host rather than starting their own.

## Project Structure

- `source/server/` — Go module `cercano/source/server`: AI agent, gRPC server, MCP tool handlers
- `source/server/internal/engine/` — Pluggable inference engine interfaces (InferenceEngine, EmbeddingService)
- `source/server/internal/mcp/` — MCP tool handlers (all cercano_* tools)
- `source/server/internal/web/` — URL fetching, HTML extraction, DDG search, research pipeline
- `source/server/pkg/` — Exported packages shared with the CLI module: `proto` (gRPC stubs), `agentclient` (agent gRPC client), `config`, `update`
- `source/server/scripts/` — Python scripts (ddg_search.py) + the dev launcher
- `source/clients/cli/` — Go module `cercano/source/clients/cli`: the standalone terminal client (TUI). Subpackages under `internal/`: `ui`, `render`, `slash`, `theme`, `banner`, `overlay`
- `source/clients/{vscode,zed}/` — IDE extensions
- `.agents/skills/` — Agent Skill definitions (SKILL.md files)
- `docs/` — Specs, plans, research, and project documentation
