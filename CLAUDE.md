# Cercano Project Instructions

## Cercano Tool Preferences

This project exposes Cercano tools via MCP. **Prefer Cercano tools for bounded
research and extraction** to conserve main-chat context. Execution follows saved
task routing and redirects; a tool name does not guarantee local or free
inference. See [routing](docs/cloud-routing.md) and
[self-development guidance](docs/agent/self-dev.md).

- **Web research**: Use `cercano_research` instead of WebSearch/WebFetch when investigating a question. It searches DuckDuckGo, fetches pages, and returns a distilled answer — using the configured route.
- **URL fetching**: Use `cercano_fetch` instead of WebFetch for reading web pages. Returns extracted text without stuffing raw HTML into the cloud context.
- **Summarization**: Use `cercano_summarize` for large files, logs, or diffs before processing them yourself.
- **Code explanation**: Use `cercano_explain` to understand unfamiliar code before bringing a summary into the main context.
- **Information extraction**: Use `cercano_extract` to pull specific info from large text instead of reading it all into context.
- **Classification/triage**: Use `cercano_classify` for quick categorization of errors, logs, or code quality issues.

**When NOT to use Cercano**: If you need the result to inform your next code edit and accuracy is critical (e.g., exact API signatures), use your own tools. Use source evidence rather than treating model summaries as verification.

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

Cercano builds as **two binaries**: `cercano` (the agent gRPC server / MCP host —
a singleton) and `cercano-cli` (the terminal UI). The CLI is a thin gRPC client;
it auto-launches `cercano agent` if no server is listening, finding the server
binary as a sibling next to its own executable. The dev launcher
(`source/server/scripts/cercano-launcher.sh`) builds both and co-locates them.

The `cercano` binary also runs one shared `cercano model-host` process that owns
llama-server and mistral.rs for the whole machine. Agent hosts launch it on
demand and it exits when idle, so several agents share one loaded model.
`CERCANO_MODEL_HOST=inprocess` keeps models inside the agent process instead
(for tests and troubleshooting). `cercano restart-after-upgrade` also relaunches
a running model host with the upgraded binary, under the same launch lock agents
use, so agents connect to the new host rather than starting their own.

## Project Structure

- `source/server/` — Go module `cercano/source/server`: AI agent, gRPC server, MCP tool handlers
- `source/server/internal/engine/` — Pluggable inference engine interfaces (InferenceEngine, EmbeddingService)
- `source/server/internal/mcp/` — MCP tool handlers (all cercano_* tools)
- `source/server/internal/web/` — URL fetching, HTML extraction, DDG search, research pipeline
- `source/server/pkg/` — Exported packages shared with the CLI module: `proto` (gRPC stubs), `agentclient` (agent gRPC client), `config`, `update`
- `source/server/scripts/` — Python scripts (ddg_search.py) + the dev launcher
- `source/clients/cli/` — Go module `cercano/source/clients/cli`: the standalone terminal client (TUI). Subpackages under `internal/`: `ui`, `render`, `slash`, `theme`, `banner`, `overlay`
- `source/clients/{vscode,zed}/` — IDE extensions
- `source/server/internal/skills/catalog/` — canonical Cercano tool skill instructions
- `.agents/skills/` and `.claude/skills/` — generated tool-skill mirrors (`cercano skills sync`); keep `plugins/skills/` distribution copies aligned
- `docs/` — Specs, plans, research, and project documentation
