# Self-development maintained-claim audit — 2026-10-22

Scope: `docs/agent/self-dev.md` only; this report records source comparison,
not runtime acceptance. Read the entire guide and the remaining-gap list in
`docs/audit-core-guides.md`. No shared ledger changes.

## Evidence recorded during comparison

- Development-mode restoration: compared `source/clients/cli/internal/ui/dev_mode_resume.go:7–18`, `model.go:3078–3090,4150–4175,4230–4250`: explicit absolute metadata restores the directory reference; ordinary metadata clears it; no kickoff replay. Server admission, persistence and rollover comparison follows below.
- Build: read both module Makefiles and `source/server/scripts/codesign-if-available.sh:1–40`. Build targets inject version and optionally sign; signing is not guaranteed and Keychain behavior is an OS/environment assertion, not an unconditional product guarantee. Launcher comparison follows below.

Completion and focused checks are recorded below as each remaining family is compared.

## Additional comparison (recorded before corrections)

- Delegation and routing: `source/server/internal/capabilities/builtins/dispatch_cap.go:20–145`, `task_control.go:27–125`, `coproc.go:17–40`, `pkg/config/routing.go:19–153,296–333`, `internal/inference/router.go:111–152` confirm asynchronous handles, owner binding, one-shot Reconnaissance, defaults/quality and redirects. Catalog availability is host-dependent, not guaranteed to every reader.
- Launcher: `source/server/scripts/cercano-launcher.sh:43–131` checks **Go files only**, builds/signs temporary siblings then renames, and terminates older agents. `install-launcher.sh:14–53` generates the copy. Direct overwrite advice conflicts with the launcher; replace it. Signing is optional.
- Timeout: `source/server/internal/capabilities/builtins/run.go:80–175,269–289`, `run_unix.go:15–48` implement 60/positive/-1, group SIGTERM on cancellation and group SIGKILL sweep on deadline only. Do not promise all descendants are reaped on ordinary cancellation.
- Trace: `source/server/internal/llm/openai/stream_trace.go:35–61`, `stream_trace_stub.go:1–12`, `stream.go:226`: sentinel checked per **fragment**, not per stream; removing it cannot override an enabled environment switch.
- Routing/lifecycle: `source/server/internal/loopcompact/wiring.go:72–170,252–295`, `cmd/cercano/main.go:305–336,657–667,721`, `internal/server/server.go:575–589,716–745` confirm shared compaction routing, startup recap selection, graceful runtime cleanup and startup orphan sweep. Unconditional orphan claim is stale.
- Smoke test: `source/server/internal/server/live_open_model_dispatch_test.go:29–105` confirms opt-in variables, dummy key, Read-only sentinel and log assertions; not executed (requires inference).

## Completion pass evidence — current source

The following supersedes the pending comparisons above; it records source-level
coverage, not live acceptance.

- **Development mode:** `internal/server/dev_mode.go:13–30` admits only explicit
  debug requests with conversation ID and absolute work directory (persistent
  store required). `internal/conversation/store.go:565–598,1170–1188` copies
  `dev_work_dir` on rollover and validates updates. CLI
  `internal/ui/model.go:3078–3090,4158–4164,4238–4245` and
  `dev_mode_resume.go:7–18` restore metadata without kickoff replay. Complete.
- **Delegation / task class / permissions:** re-read
  `internal/capabilities/builtins/dispatch_cap.go:20–145`,
  `task_control.go:27–125`, `coproc.go:17–40`, `pkg/config/routing.go:50–155`,
  `internal/inference/router.go:111–152`. Defaults, quality, host ownership,
  asynchronous results and wait-versus-stop distinction match. Complete at the
  documented capability-contract boundary; host tool availability is qualified.
- **Layout/build:** both module Makefiles and
  `scripts/codesign-if-available.sh:17–33`, `cercano-launcher.sh:43–131`,
  `install-launcher.sh:14–53` compared. Staleness scans all `.go` files, not
  Git-tracked files only. Optional signing and OS-dependent Keychain behavior
  are qualified. Complete; no build/install/restart performed.
- **Logs:** `pkg/agentclient/client.go:175–215` uses `os.TempDir()` and sibling
  binary before PATH; `cmd/cercano/main.go:87–103,1782–1788` configures dispatch
  and crash logs. Crash capture can fail; temp-directory wording corrected.
- **Databases:** `internal/conversation/schema.sql:1–75`,
  `store.go:362–410,439,598–642,985` verify paths, WAL, schema/migrations and
  Unix seconds. `internal/telemetry/telemetry.go:156–191,235–254,333` verifies
  WAL/DATETIME and usage aggregation; `cmd/cercano/main.go:509,906,1749` and
  `internal/mcp/server.go:317` verify location and stats entry points. Complete.
- **Config surfaces:** `pkg/config/config.go:758–764`,
  `cmd/cercano/main.go:482–483`, `internal/mcp_host/config.go:44–77`, CLI
  `internal/uiconfig/uiconfig.go:16–30` verify defaults and overrides. Complete.
- **Troubleshooting / timeout:** runtime lookup is in
  `internal/localruntime/llamaserver/provider.go:791–826,1452–1458`; errors are
  diagnostic leads, not an external compatibility guarantee. Timeout contract
  is in `internal/capabilities/builtins/run.go:80–175,269–289` and Unix group
  cancellation in `run_unix.go:15–48`. Complete, subject to stated platform and
  descendant-cleanup limitations.
- **Tracing:** `internal/llm/openai/stream_trace.go:35–61` and
  `stream_trace_stub.go:1–12` verify build tag, startup environment switch,
  per-fragment sentinel and output. Complete; external incident is historical.
- **Live smoke:** `internal/server/live_open_model_dispatch_test.go:29–105`
  verifies opt-in gates, dummy key, Read-only grant, sentinel and log assertions.
  Complete source comparison only; no model inference run.
- **Background routing / bounce:** `internal/loopcompact/wiring.go:252–295`,
  `cmd/cercano/main.go:309–314,657–667,718–731`,
  `internal/server/server.go:575–589,716–745`, and
  `internal/compactiongen/compactiongen.go:209–292` verify compaction selection,
  recap startup selection, cleanup attempts/orphan sweeps and success logging.
  `pkg/agentclient/client.go:175–215` verifies auto-launch does not rebuild.
  Complete source comparison, not process-reconnect or OS acceptance.

Unless explicitly prefixed otherwise, source paths above are relative to
`source/server/`; CLI paths are relative to `source/clients/cli/`.

## Final readback corrections

Re-read the full guide in bounded ranges and the core-guide gap entry. Current
source comparison reconfirmed dispatch admission/routing, launcher/signing,
storage/config paths, timeout and tracing contracts. Corrected remaining
`tracked .go` wording (launcher uses `find`, not Git), the auto-launch temp path
(`os.TempDir`, not an unconditional `$TMPDIR`), and optional crash capture.
Qualified streaming fragmentation as a possible cause rather than an unsupported
frequency claim. Evidence: `source/server/scripts/cercano-launcher.sh:43–48`,
`source/server/pkg/agentclient/client.go:179–185`,
`source/server/cmd/cercano/main.go:1782–1788`, and
`source/server/internal/llm/openai/stream_trace.go:35–61`.

Task-stop precision: `task_stop` requests cancellation rather than proving
terminal completion; consult `source/server/internal/capabilities/builtins/task_control.go:27–125`.

## Publication scope and limitations

This publication preserves the saved source-comparison evidence above; it is not
an audit relaunch or live validation. The publication source-check batch could
not complete its searches because `rg` was unavailable, so not every changed
claim was independently revalidated in this pass. No builds, tests, installs,
model inference, live database queries or runtime acceptance checks were run.
Broader core-guide coverage remains incomplete; no shared ledger was changed.
