# cercano-cli end-to-end tests

Black-box tests that run the real `cercano-cli` and `cercano agent` binaries in a
pseudo-terminal, press keys and assert on the rendered screen. They use
[`@microsoft/tui-test`](https://github.com/microsoft/tui-test) (pinned to
0.1.0) with Node's built-in `node:test` runner.

```bash
cd test/e2e
npm ci
npm run build   # builds .bin/cercano and .bin/cercano-cli side by side
npm test
```

Requires Go (for the build), Node 20+, and `lsof`/`ps` (for auto-launch
cleanup). Runs on macOS and Linux. CI uses Node 22.13.0.

Run one file or one test:

```bash
node --test specs/chat.test.mjs
node --test --test-name-pattern="Esc cancels" specs/chat.test.mjs
```

## Isolation

Each test gets a sandbox (`helpers/sandbox.mjs`), so the suite never touches
`~/.config/cercano` or a running agent:

- `HOME`, `XDG_CONFIG_HOME` and `TMPDIR` point into a fresh `/tmp/ce2e-*`
  directory. It sits under `/tmp` because the agent's worker sockets live
  under `$TMPDIR`, and macOS caps Unix socket paths at 104 bytes.
- The default profile's agent gets a free `CERCANO_PORT`, never the default
  50052. A test that passes `profile` selects that profile through
  `CERCANO_PROFILE` and sets no port: each profile's agent takes an ephemeral
  port and publishes it in `profiles/<name>/agent.json`, so several profiles can
  run in one sandbox.
- A `config.yaml` is seeded in the profile's directory,
  `~/.config/cercano/profiles/<name>/`, with `locus_mode: open_only` and
  `open_runtime: ollama`, unless the test opts out to exercise the first-run
  wizard. `sandbox.writeConfig("", { legacy: true })` seeds the pre-profile
  location `~/.config/cercano/config.yaml` instead, for migration tests.
- `CERCANO_MODEL_HOST=inprocess` keeps local models in the agent, so no shared
  `cercano model-host` is launched and none outlives a test.
- `OLLAMA_URL` points at `helpers/fake-ollama.mjs`, which streams a scripted
  reply and records requests, or at an unused port when the test never chats.
  No cloud keys are set.
- The OS keychain is not sandboxed, so specs never save keys or sign in.

Most specs start `cercano agent` themselves (`helpers/agent.mjs`) and stop its
whole process group afterwards. `server-discovery.test.mjs`,
`profiles.test.mjs` and `profile-migration.test.mjs` cover the CLI's own
auto-launch path; an auto-launched agent logs to
`profiles/<name>/logs/agent.log` in the sandbox. Their fallback cleanup
(`helpers/owned-agent.mjs`) treats port listeners and `agent.json` records only
as candidates: it verifies the exact agent command, sandbox HOME/TMPDIR/port,
profile, and auto-launch marker before signalling. It sends SIGTERM, waits up
to five seconds, then rechecks ownership and process start time before
SIGKILL. Unverifiable or unrelated processes are never signalled; an
unsupported ownership probe can therefore leave a failed run's agent for manual
inspection. Regression tests cover ownership rejection, escalation, identity
changes, and a real sandbox agent.

## The per-profile agent hosts flag

`specs/per-profile-flag*.test.mjs` cover `CERCANO_PER_PROFILE_AGENTS` absent,
`"false"`/`"0"` and `"1"`/`"true"` through the real binaries, including
`cercano --mcp` over JSON-RPC (`helpers/flag.mjs`). They set the flag per
process rather than relying on the sandbox default (`""` for the default
profile, `"1"` with `profile`), and read each process's environment back
(`helpers/procs.mjs`) to check the state it really ran with. MCP servers get a
fresh release-check cache and a dead HTTP(S) proxy (loopback is never
proxied), so they make no external request. Leftover processes are found by
the sandbox's own HOME and TMPDIR on a suite binary, and only those are
signalled.

The upgrade tests run builds from before the flag (f79bf87a) and before
profiles (c23f4656). Build them, offline, with:

```bash
./scripts/build-historical.sh
```

Without them those tests are reported as skipped; set
`E2E_REQUIRE_HISTORICAL=1` to fail instead.
`specs/per-profile-flag-regressions.test.mjs` guards fixed defects: a refused
named profile must not migrate pre-profile data or stop a pre-profile agent,
and `cercano --mcp` must keep standard output to JSON-RPC across real tool
calls. The upgrade-restart decisions are covered by the Go transition test
(`source/server/cmd/cercano/per_profile_flag_transition_test.go`), which needs
the same historical builds (`CERCANO_REQUIRE_HISTORICAL=1` makes them
required).

## Writing a spec

```js
import { test } from "node:test";
import { withCli, ready } from "../helpers/cli.mjs";

test("does the thing", async (t) => {
  await withCli(t, { ollama: { reply: "hi" } }, async ({ term, ollama }) => {
    await ready(term);
    await term.submit("hello");
    await term.getByText("hi").expect();
  });
});
```

`withCli` options: `args`, `agent` (start an agent, default true),
`seedConfig` (default true), `ollama` (true or `{ reply, chunkDelayMs }`),
`cols`/`rows` (default 120×40), `bin`, `env`, `profile` (a named profile on an
ephemeral port) and `before` (an async `(sandbox) => {}` that seeds files before
the agent and CLI start). The callback also receives `defer(cleanup)`: deferred
cleanups run after the CLI closes and before the sandbox is removed, which is
where a spec stops an agent the CLI auto-launched. `runCli(t, sandbox, { label,
args })` starts another CLI in the same sandbox; the spec closes it.

Use `getByText(...).expect()` to wait for content. It polls until the 15s
timeout, so avoid fixed sleeps. Prefer structural assertions to
`expectSnapshot`: the renderer's cursor-movement bytes vary between frames,
which makes full-screen snapshots brittle.

## When a test fails

The failing screen, the agent log and the fake-Ollama request paths are printed
as test diagnostics. A full tui-test trace (`trace.html`, `session.cast`,
`current.svg`, `timeline.json`) is written to `artifacts/traces/`. CI uploads
`artifacts/` as `cli-e2e-traces`. Set `E2E_KEEP_SANDBOX=1` to keep the sandbox
directories for inspection.

Tests marked `{ todo }` document known UI issues; they run and report but don't
fail the suite.
