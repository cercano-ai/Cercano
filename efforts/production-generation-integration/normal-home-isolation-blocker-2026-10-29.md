# Normal-HOME acceptance isolation audit: stopped before launch

## Outcome

**Blocked, not fixed or accepted.** Ordinary HOME was not used to launch an application or native credential probe in this follow-up. No credential acquisition result for an actual generation executable was obtained. No executable was built/published, so there is no new generation identity to report. The earlier helper result is not generation A's result.

Read `AGENTS.md`, the native-Keychain observation and real-cloud observation of 2026-10-29, the acceptance HTTP harness and its tests, and the source boundaries below. The documented HOME experiment was not repeated. No credentials, account identities, HTTP headers, or native result data were inspected.

## Audit coverage and precise blocker

This is a **partial audit**, sufficient to reject changing only HOME or XDG variables, not an exhaustive isolation proof.

| Area | Source and finding |
| --- | --- |
| Launcher / host records, token and log | `source/server/cmd/cercano/development_launch.go:112-337`: explicit private state/runtime validation, prepared database/config, exact generation/digests, state-local hosts/selections/lock/start-intent/token/log. The host child inherits `os.Environ()`; this is not a write sandbox. |
| Configuration / permissions | `source/server/cmd/cercano/development_host_production.go:81-158`: validated admission selects config and conversation authority; permissions are adjacent to config, missing policy stays strict; credentials are read-only. `:280-286` enables writable settings with the explicit config path. Full settings-handler confinement was not qualified here. |
| Prepared conversations | Same constructor `:285-295` supplies admitted database/coexistence and exact prepared store. A full storage/sidecar and handler traversal was not completed in this follow-up. |
| Diagnostics / accounting | Same constructor `:253-286`: host diagnostics under `state/hosts/host`, explicit routing/failure files. `source/server/cmd/cercano/telemetry_startup.go:28-35` selects telemetry adjacent to admitted config. `source/server/internal/llm/anomaly.go:17-45` separately defaults to a content-bearing `HOME/.config/cercano/stream-anomalies.jsonl`; it requires an explicit private `CERCANO_ANOMALY_LOG`. |
| Worker executable / socket resources | `source/server/internal/worker/policy.go:59-112` pins exact generation/manifest and creates a host namespace beneath explicit short resource root, excluding legacy sweep. `spawn.go:371` explicitly leaves worker environment inherited. This resource policy alone does not relocate HOME-derived tool writes. |
| MCP / capabilities | `source/server/cmd/cercano/development_host_production.go:327-336` installs production services and starts MCP with admitted config directory. `production_composition.go:122-146` installs the ordinary capability graph, and `:162-187` constructs/starts its manager. Explicit manager configuration location is not confinement of every tool/child-process write. |
| **Concrete HOME write escape** | `source/server/internal/capabilities/builtins/web_common.go:26-31` calls `web.EnsureSearchScript("")`. `source/server/internal/web/script.go:22-28,36-77` resolves that empty path directly from `os.UserHomeDir()` to `.config/cercano/scripts`, creates directories/temp files and renames `ddg_search.py`. There is no per-invocation directory supplied on this path. MCP handlers also call it with an empty path (`internal/mcp/server.go:751,1039`). Neither the host's `--state` nor XDG overrides redirect this implementation. This is a reachable-tool escape, **not a claim that host startup itself executes web search**. |
| Catalog / enterprise | `production_catalog.go:56-95` retains activation admission before cache factories/background work and locates cache adjacent to admitted config. `production_enterprise.go:38-43` rejects unsupported prepared enterprise before construction. These guards were not changed; they must not be bypassed to obtain a probe. |
| Other HOME-derived state / child processes | Not exhaustively qualified. In particular, explicit resource roots do not by themselves confine local-runtime caches/model writes, tool work directories, external MCP processes, or all ordinary server handlers. No assurance of all reachable writable paths is claimed. |

**Stop reason:** the existing launch setup cannot yet guarantee every reachable writable application path is private with normal HOME. A normal-HOME launch with only private config/state/runtime and XDG overrides would leave at least the concrete script-materialization path above outside the private tree. Replacing HOME would repeat the documented Keychain-context failure, not solve isolation. Per the user's fail-closed instruction, stop before native/app launch rather than rely on an instruction not to invoke tools.

The existing `realcloudacceptance` transport is an inference-budget harness, not a credential-only execution mode (`internal/llm/responses/acceptance_run.go:95-135`). It must not be used as evidence that a generation identity can safely acquire credentials without inference. A bounded diagnostic branch inside the actual generation remains unimplemented; any future branch must execute before ordinary composition, restrict the existing profile, disable native interaction, avoid enumeration/refresh/write/inference, sanitize status, and be time-bounded. If source changes, its published identity must be recorded as a new generation, not old A.

## Local verification only

Read `acceptance_run_test.go`: its HTTP paths use injected `acceptanceWire` fake transports; the default transport test only checks construction/options. The subprocess test executes only the test binary to update a temporary counter.

From `source/server`:

```text
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off \
CGO_CFLAGS=-Wno-deprecated-declarations \
go test -tags realcloudacceptance ./internal/llm/responses \
  -run '^TestAcceptance' -count=1 -timeout=60s
```

PASS, exit 0, package 3.326 seconds (tool elapsed 4.301 seconds). These are existing harness regression tests, not tests of a completed normal-HOME isolation implementation. No native credential backend or provider inference was exercised by this command.

## Changes and safety boundary

Only this evidence file was authored. No application source/policy, live config/history, installation, existing process, Keychain item, ACL or search list was changed. No dialog was enabled or approved, no login/refresh/Set/Delete, no provider calls, no push/merge/install. The pre-existing modification to `efforts/production-generation-integration/plan.md` was left untouched and excluded from the evidence checkpoint.
