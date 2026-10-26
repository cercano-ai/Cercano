# Agent README startup and headless source audit

## Scope and method

Compared only startup/launcher and headless-command claims in `docs/agent/README.md`: setup steps 2, 3 and 6; the headless capability bullet; Quick start launcher, interactive invocation and headless examples. Relevant README text was read once. Baseline supplied by the parent: `ccf1d9c0d3d434fcbac81286b64934e09bb3a315`, branch `docs/publicity-refresh`.

This is source comparison, **not runtime acceptance**.

## Section-level evidence and results

### Setup: Build the dev launcher / Quick start: Install the dev launcher

- `source/server/scripts/cercano-launcher.sh:28-39` exports `CERCANO_REPO` and co-locates the server and CLI under `$HOME/bin/.cercano-libexec`.
- `source/server/scripts/cercano-launcher.sh:43-49,71-88` rebuilds missing binaries or binaries older than relevant `.go` files; `:90-123` checks process start times against the server binary and attempts to terminate stale agents.
- No change to the rebuild/stale-agent summary or generation instructions. These are summaries, not a guarantee that all dependency changes trigger rebuilds or that every termination succeeds.

### Setup: Run it / Quick start: launcher and interactive invocation

- `source/server/scripts/cercano-launcher.sh:125-127` unconditionally execs the CLI with original arguments. There is no stdin/TTY dispatch.
- `source/server/cmd/cercano/main.go:918-928,1008-1020` registers `agent` and makes the bare server binary run in foreground server mode.
- `source/server/pkg/agentclient/client.go:90-118,146-167` first attempts a connection, then auto-launches when connection fails. `:174-195,202-214` launches the discovered binary with `agent`, checking its executable sibling before PATH.
- Corrected the TTY-conditioned launcher description to unconditional CLI execution. Replaced the misleading dev-launcher `cercano agent` instruction with the explicit libexec server path, retaining `cercano agent` for installations where that name resolves to the server binary.
- Replaced the unconditional `:50052` startup wording with connection-based discovery wording. The reviewed client accepts a supplied address; headless builds its address from configured `cfg.Port` (`headless.go:75-93`). The default port value itself was not audited.
- Interactive `cercano` invocation and installed server/CLI distinction retained.

### Setup: Or run it headless / Quick start: Headless / scripted mode

- `source/server/cmd/cercano/main.go:935-939` registers `run` on the server binary. The dev launcher merely forwards arguments to the CLI (`cercano-launcher.sh:127`); these sources do not establish CLI support for server subcommands.
- Corrected both example blocks to invoke `"$HOME/bin/.cercano-libexec/cercano" run` for dev-launcher installations; explicitly retained `cercano run` for release installations where it is the server binary.
- `source/server/cmd/cercano/headless.go:38-73` parses `--auto-allow`, `--conv`, and the prompt; `:23-27,103-107` preserves explicit conversation IDs and otherwise creates a new one. Example flags and continuation behavior retained.
- `headless.go:94-111,160-176` returns exit 1 for connection/stream errors, error events, or a completed turn with no assistant output. Added the previously omitted exit 1 description in both sections.
- `headless.go:136-149` automatically denies permission requests unless `--auto-allow` is set, recording exit 3. Replaced “you/user denied a confirm gate” with automatic denial by default. A later error event can overwrite exit 3 with exit 1; the README remains a concise summary rather than an exhaustive precedence specification.
- `headless.go:117-135,150-168` sends assistant text to stdout and diagnostics/notices/errors to stderr, printing final text only if no deltas were seen. Kept the output split, explicitly including stderr errors.
- Headless capability bullet retained: server command registration supports the generic `cercano run` form when `cercano` resolves to that binary.

## Limitations and checks

- Did not audit prerequisites, model/cloud setup, permission-mode policy, slash commands, architecture, or other README topics. Reading surrounding text did not constitute auditing it.
- Did not inspect installer/Makefile, release packaging, CLI argument handling, config defaults, or other startup implementation files. Clone-path baking, PATH installation, release packaging, and actual CLI behavior remain outside this comparison. The explicit dev server path comes directly from the launcher.
- No binaries executed, builds, runtime tests, network requests, installs, restarts, paid calls, application/dependency changes, shared-ledger edits, commits, or pushes.
- Final scoped validation: whitespace/diff and local Markdown-link checks are performed separately after writing this report; their observed results are returned with the audit handoff. External links are not fetched.
