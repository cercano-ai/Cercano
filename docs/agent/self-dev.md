# Self-Development Guide

How to build, test, and inspect Cercano when working on Cercano itself. This
doc is requested by the kickoff prompt on entering development mode (`/d`), alongside
`docs/features/cli/README.md` (CLI track status + outstanding work) and
`docs/agent/README.md` (agent architecture).

## Development mode survives resume

Entering `/d` records the repository directory as explicit conversation metadata when its request is admitted. Restarting the CLI and reloading that conversation restores the working-directory override, completion/tool directory, and dev/debug controls without resending the kickoff prompt. Progressive resume includes this state in its first page, before input is enabled; buffered resume restores it from conversation metadata. Conversation rollover carries it forward, while loading an ordinary conversation clears it.

Older conversations have no recorded dev-mode metadata. Enter `/d` once after upgrading to record it; subsequent restarts restore it automatically. Dev mode is never inferred from transcript text or from a conversation merely having a project directory.

## Delegate to open models — this is the point of Cercano

Prefer bounded delegation for routine reconnaissance when the host exposes it and
its route, cost and permissions fit the task. Local-model quality depends on the
model and workload; inspect the returned evidence rather than assuming parity
with a frontier model. Follow the current host's tool catalog and task constraints.

Cercano registers delegation as native agent capabilities as well as MCP-facing
capabilities; skill descriptions are not proof that a particular host grants them.

| Tool | Shape | Runs on | Use it for |
|---|---|---|---|
| `dispatch` (alias `workflow`) | agentic sub-agent (bounded tool loop over a granted toolset) | saved task class → redirects → locality policy | tracing a code path, "how/if does X happen," understanding a class, finding something in a subsystem, any read-heavy recon that returns a distilled answer |
| `explain` | one-shot text analysis | Reconnaissance route | explaining a file/snippet you already have in hand |
| `summarize` | one-shot text analysis | Reconnaissance route | condensing a long file or output |
| `extract` | one-shot text analysis | Reconnaissance route | pulling specific facts out of text |
| `classify` | one-shot text analysis | Reconnaissance route | bucketing text/files |

Public `dispatch`/`workflow` returns an accepted task handle immediately, not the
finished report. Use `task_inspect` or `task_wait` before relying on the child's
answer; use `task_stop` to request cancellation, not as proof of terminal completion. See [independent tasks](independent-tasks.md).


**Canonical delegation example.** Instead of running fifteen Grep/Read calls
yourself to chase a code path, call `dispatch` with a concrete intent and a
read-only grant:

```json
{
  "class": "reconnaissance",
  "task": "Figure out how/if model reloading happens when the backend changes. Trace the code and return the full code path, the relevant code snippets, and their file:line locations.",
  "tools": ["Read", "Grep", "Glob"]
}
```

The sub-agent uses the configured Reconnaissance route (Local/Light by default), which the user can override. Do not assume it is free or local after a redirect.

### Task class, quality, and permissions are independent

Choose `class` at the semantic entry point:

- `reconnaissance`: narrow read-only tracing, extraction and summaries (Local/Light).
- `mechanical_development`: well-specified edits (Local/Standard).
- `investigation`: open-ended diagnosis (Secondary/Standard).
- `implementation`: substantial implementation (Secondary/Standard).
- `review`: standalone adversarial review (Secondary/Standard).
- `research`: source gathering, query generation and synthesis (Secondary/Standard).
- `git_land`: the existing landing workflow including its nested review (Local/Premium).
- `watchdog`: normal Watchdog routing (Local/Standard); no special placement exemption.

Chat is Primary/Standard; omitted dispatch class uses Default dispatch (Secondary/Standard). Unknown explicit classes are rejected. Prefer omitting `tier` so the chosen class's saved quality applies. Explicit `light`, `standard`, or `deep` changes quality only; it never chooses a destination, class or permission grant. Light uses the persisted `economy` value.

Secondary/Local redirects resolve before placement and model selection. Only the final destination's models, credentials and backup chain apply. Saved source assignments remain unchanged. Locus policy still prohibits forbidden placement; final Secondary cannot fall through to Primary/Local. Tool permissions remain the explicit grant and approval system, independent of model quality. See [the routing guide](../cloud-routing.md).

The old RoleCoproc/default-light delegation policy is retired. Text analysis uses Reconnaissance; the explicit `local` tool retains its existing local-offload intent. New first-party callers must pass a task class; do not infer one from role, prompt text or tool/source names.

## Layout & build

Two binaries from two Go modules:

| Binary | Module | Role |
|---|---|---|
| `cercano` | `source/server` | agent gRPC server, MCP host — a singleton |
| `cercano-cli` | `source/clients/cli` | terminal UI, thin gRPC client |

```bash
# Server (agent + MCP)
make -C source/server build
(cd source/server && go test ./... -count=1)

# CLI (separate Go module; depends on server pkg/ via replace directive)
make -C source/clients/cli build
(cd source/clients/cli && go test ./... -count=1)
```

**Prefer `make build`** (in each module) over bare `go build`: the make targets
inject the Git-derived build version (`git describe --tags --always --dirty`,
with the leading `v` stripped; `VERSION` overrides it) and invoke
`source/server/scripts/codesign-if-available.sh`. Signing is conditional on an
available Developer ID identity; `CERCANO_CODESIGN_ID` selects one (`none`
disables signing). A stable identity is intended to preserve macOS Keychain
access across rebuilds, but prompts depend on OS and Keychain configuration.
The launcher uses the same version/signing convention. Bare `go build` without
version ldflags reports `dev` and does not run this signing helper.

**Development launcher:** the source launcher publishes immutable generations
and uses separate prepared development state; it does not kill singleton agents.
Follow [development generations](development-generations.md) for one-time
build-only bootstrap and `prepare-development`. Preparation carries forward
`~/.config/cercano/config.yaml` privately without modifying the original or
copying databases/native credentials; later builds reuse the development copy.
The source launcher's default state is `~/.config/cercano-development`;
`CERCANO_DEVELOPMENT_STATE` selects a custom prepared state. It still refuses an
unprepared store rather than silently creating or migrating one. Template changes
require an explicitly reviewed installed-launcher replacement; the no-clobber
installer only creates a new destination (`make launcher LAUNCHER_DEST=...`).

## Logs

| Path | What it is |
|---|---|
| `<os.TempDir()>/cercano-server.log` | stdout/stderr of an auto-launched agent (`$TMPDIR` on Unix when set, otherwise `/tmp`) |
| `~/.config/cercano/crash.log` | crash reports when crash capture initializes successfully |
| `~/.cercano-dispatch.log` | MCP dispatch-mode diagnostics |

The auto-launch log path comes from `pkg/agentclient/client.go` (it prints as
"auto-launched agent server (log: …)" when auto-launching). When debugging agent
behavior, check the server log first — tool-loop iterations, provider errors,
and permission diagnostics may appear there; manually started agents use their
configured stdout/stderr instead.

## Databases

Both stores enable SQLite WAL mode. Use read-only queries (for example,
`sqlite3 -readonly`) when inspecting live data. For a consistent copy use
SQLite's backup facility; copying a live database and its sidecars separately
is not an atomic snapshot. Conversation timestamps below are integer Unix
**seconds**; telemetry timestamps are stored as SQLite DATETIME values.

### `~/.config/cercano/conversations.db`

Override with `$CERCANO_CONVERSATIONS_DB`. Schema sources of truth:
`source/server/internal/conversation/schema.sql` and migrations in `store.go`.
Selected tables: `conversations`
(id, title, project_dir, recap, timestamps), `turns` (role, content,
token counts, created_at), `conversation_compaction` (derived summaries).
Tool-calling turns carry an ordered Anthropic-wire-shape block array in the
`content_json` column; text-only turns use `content`.

```bash
# Ten most recent conversations
sqlite3 -readonly ~/.config/cercano/conversations.db \
  "SELECT id, title, datetime(last_turn_at,'unixepoch') FROM conversations \
   ORDER BY last_turn_at DESC LIMIT 10;"

# A conversation's turns, in order (truncate long bodies)
sqlite3 -readonly ~/.config/cercano/conversations.db \
  "SELECT role, substr(coalesce(nullif(content,''), content_json),1,120) \
   FROM turns WHERE conversation_id='<id>' ORDER BY created_at;"

# Turns whose tool calls mention a given tool
sqlite3 -readonly ~/.config/cercano/conversations.db \
  "SELECT conversation_id, datetime(created_at,'unixepoch') FROM turns \
   WHERE content_json LIKE '%\"name\":\"Edit\"%' ORDER BY created_at DESC LIMIT 20;"
```

### `~/.config/cercano/telemetry.db`

Usage metrics: local/cloud requests, token counts, per-tool and per-day
breakdowns. Read via the `cercano_stats` MCP tool or `cercano stats` CLI
command; raw SQL works too (`.tables` to explore — the schema is small).

## Config surfaces

Default locations are under `~/.config/cercano/`: `config.yaml` (server: models, endpoints,
cloud provider), `permissions.yaml` (permission mode + allowlist), `ui.yaml`
(CLI), and `mcp.yaml` (hosted MCP servers; legacy `mcp.json` is also
imported, with YAML winning collisions). The server host loads from the config
directory, not a per-project `.cercano/mcp.yaml` override. CLI UI config honors
`CERCANO_UI_CONFIG` and `XDG_CONFIG_HOME`.

## Where outstanding work is tracked

- `docs/features/cli/README.md` — task rollups per phase, deviations from
  plan, and the "still outstanding" list for the CLI track.
- `README.md` (repo root) — overview and links to feature/status documentation.
- `docs/features/` and `docs/agent/` — per-feature `design.md` / `plan.md`
  pairs; historical unchecked boxes are not a reliable current backlog. Confirm
  implementation and current tracking before treating a plan step as outstanding.

## Troubleshooting local models & the agent lifecycle

Historical debugging notes (2026-07-07), updated for current source. These are
diagnostic leads, not guarantees about external runtimes or model formats. When a
background job (recap, compaction, watchdog) fails, triage by the error's
*shape* in the server log:

| Log error | Meaning | Where to look |
|---|---|---|
| `model "X" not found in configured model_dirs` | name resolution — the requested name matches no GGUF (`matchesModel` in `internal/localruntime/llamaserver/provider.go`; bare names alias `<name>-latest` stems) | tier values in `config.yaml` vs actual filenames |
| `llama-server exited during startup: exit status 1` | startup failed; inspect stderr and reproduce the actual invocation (below) | GGUF/llama.cpp compatibility |
| `chat error (status 400) … exceeds the available context size` | request exceeds the runtime context budget; account for prompt, tool schema and output reserve (no fixed context size guarantees success) | `llama_server.context_size` |

Reproduce a spawn failure outside the agent. A stock macOS shell may lack coreutils `timeout`,
but you do **not** need the old background-and-kill dance from a Cercano tool
call: the `RunCommand` tool (legacy alias `Bash`) enforces `timeout_seconds` itself (default 60), signalling
the command's process group on Unix and returning partial output on timeout. Pass
`timeout_seconds` rather than hand-rolling a cap.

`timeout_seconds` values: omit it (or pass `0`) for the 60s default, a positive
number for that many seconds, or `-1` for **no timeout** — use `-1` for
genuinely unbounded work such as a cold-cache build, a large migration, or a
long sweep, where guessing a bound risks killing legitimate work. Any other
negative is rejected, so a typo'd `-60` cannot silently mean "forever".
Unbounded is not unstoppable: `-1` only removes the deadline, and cancelling
the turn still sends SIGTERM to the Unix process group. A deadline also triggers
a final group SIGKILL sweep; cancellation alone does not guarantee cleanup of
SIGTERM-resistant descendants. `RunCommand` takes argv, not an implicit shell;
use an explicit shell for pipelines or background jobs.

The background + kill form below is for running this by hand in a plain shell:

```bash
/opt/homebrew/bin/llama-server -m ~/.cercano/models/<file>.gguf \
  --host 127.0.0.1 --port 18099 --ctx-size 16384 --gpu-layers auto \
  > /tmp/lls.log 2>&1 & sleep 5; kill %1; grep -iE 'error|missing' /tmp/lls.log
```

Historical compatibility report: an Ollama-sourced qwen3-coder-next blob failed
with `missing tensor 'blk.0.ssm_dt.bias'` on the upstream builds tried at the
time. This is not a current compatibility matrix. Check the exact artifact and
runtime version; a compatible conversion or runtime may be needed. Neither a
runtime upgrade nor a different download source alone guarantees a fix.

### Tracing raw tool-call streaming (`cercano_streamtrace`)

When an OpenAI-compatible server (llama-server, vLLM, etc.) produces wrong or
missing tool calls, one possible cause is *how it splits one tool call across
streaming fragments* — the name, id, and argument JSON can arrive on different
`data:` chunks, and the reassembly in `internal/llm/openai/stream.go` has to
tolerate it. Don't infer that wire shape from second-order symptoms; observe it.

A build-tagged tracer dumps every raw tool-call fragment to stderr. It is
**absent from normal builds** (a no-op stub the compiler inlines away — zero
`os.Stat`/env cost on the hot path), so it must be compiled in explicitly:

```bash
cd source/server
tmp="$HOME/bin/.cercano-libexec/cercano.new.$$"
go build -tags cercano_streamtrace -o "$tmp" ./cmd/cercano && \
  scripts/codesign-if-available.sh "$tmp" cercano && \
  mv -f "$tmp" "$HOME/bin/.cercano-libexec/cercano"
# Restart the agent only when it is safe to interrupt active work.
```

Once running a trace-tagged binary, enable it at runtime with EITHER:

- `CERCANO_TRACE_OPENAI_STREAM=1` in the server's env (read once at start), or
- `mkdir -p ~/.cercano; touch ~/.cercano/trace-openai-stream` — checked per fragment, so it
  toggles on the already-running singleton server with no restart (remove the
  file to turn it off only if the environment switch is not enabled).

Output, one line per fragment, in the server log/stderr:

```
[openai-stream-trace] tool_call fragment idx=0 id="abc" name="Read" args="{"
[openai-stream-trace] tool_call fragment idx=0 id="" name="" args="\"path\":"
```

Historical incident (2026-08-12): tracing the GLM-4.5-Air `called=[]` case
reportedly ruled out deferred-name streaming and exposed lost tool-use evidence.
This anecdote does not establish the wire format of current model/runtime builds.
Trace output can contain sensitive tool arguments; handle and remove logs accordingly.

### Opt-in live open-model dispatch smoke

Most dispatch/sub-agent tests are hermetic and should run in normal `go test
./...`. To validate the real local/open-model path on a configured development
machine, use the skipped-by-default live smoke test:

```bash
cd source/server
CERCANO_LIVE_OPEN_MODEL_TEST=1 \
CERCANO_LIVE_OPEN_MODEL_BASE_URL=http://127.0.0.1:8080/v1 \
CERCANO_LIVE_OPEN_MODEL_MODEL=glm-4.5-air \
go test ./internal/server/ -run TestLiveOpenModelDispatch_ReadToolSmoke -count=1 -v
```

Optional: set `CERCANO_LIVE_OPEN_MODEL_API_KEY` if the endpoint requires one;
otherwise the test uses a dummy `local` key. The test creates a temp file with a
sentinel token, grants only `Read`, runs an agentic dispatch against the local
OpenAI-compatible provider, and asserts both the returned text and dispatch log
show a real tool-grounded run (`called=[Read]`). It is skipped unless
`CERCANO_LIVE_OPEN_MODEL_TEST=1` is set, so CI and normal development never
require a local model.

Model resolution is task-specific, not one startup-only local-model mapping.
Compaction uses shared `internal/loopcompact` routing (default Secondary/Light);
`compaction.summarizer_model` applies only to explicitly Local, resolved-local
execution. Watchdog uses its saved task assignment (default Local/Standard).
The recap generator is wired at startup with `fast_light_text.open`; local chat
uses `open_model`. Diagnose the effective route and selected model rather than
assuming every background job uses the local fast/light model. See
[compaction routing](compaction-routing.md) and [routing](../cloud-routing.md).

Agent-bounce pitfalls:

- Graceful server shutdown attempts to stop managed runtime instances; startup
  also sweeps owned llama-server orphans. Hard kills or failed cleanup can still
  leave children with old settings. Inspect ownership before terminating any process.
- Auto-launch executes the server sibling of the running CLI, falling back to
  `PATH`; with the dev launcher this is `~/bin/.cercano-libexec/cercano`.
  It does not itself rebuild. After source changes, run the installed dev
  launcher or explicitly build/install before trusting a bounce.
- Install rebuilt signed binaries via a temporary sibling and rename (as the
  launcher does), not by overwriting an executing binary in place. This avoids
  the signature-cache problems noted in historical macOS debugging; exact OS
  behavior is environment-dependent.
- Verify a bounce actually happened: compare the agent PID/start time
  (`ps -o pid,lstart -p $(lsof -tiTCP:50052 -sTCP:LISTEN)`) against your kill,
  and count startup banners in the server log. A phantom "killed" echo from a
  mangled PID extraction cost a full debugging round.
- Background compaction logs `pass ok` with before/after token counts after
  saving changed state; it also logs deferred, failed and no-op passes.
  Recap success is visible in `conversations.db` (`recap`,
  `recap_updated_at`).
