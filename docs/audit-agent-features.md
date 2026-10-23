# Agent feature semantic source audit

Date: 2026-10-22. Scope: all **11** current `docs/agent/features/*.md` files, including the index. This is a bounded source audit, not an inventory-only review or a live-provider certification. All guides were read in full (split reads where combined output was truncated). No current guide was exempted as historical. Only the feature guides and this audit record are in the edit scope; pre-existing changes elsewhere were left alone.

## Per-file evidence and disposition

Source paths below are repository-relative. Named functions identify implementation evidence, not merely matching terminology. Verification refers to the focused commands below; passing tests do not independently establish every prose claim.

### 1. `README.md`

Reviewed every feature summary and all ten guide links against the traces below. Routing, delegation, workflows, context, metrics, runtime, cloud providers, terminal UI, persistence, and client/server descriptions have corresponding implementations. Qualified saved-session resumption by configurable retention aging. The former co-processor link is background navigation, not an exemption for any current feature. Supporting command/permission/MCP links are navigation rather than a claim of exhaustive coverage here.

### 2. `advanced-metrics.md`

`source/server/internal/telemetry/metrics.go` (`MetricsBounds`, `QueryTokenMetrics`, `metricsWarnings`, line 81) implements date bounds, provider/model/source filters, separate attempts/external populations, unknown-counter and accounting-health warnings, and exclusion of legacy records. External queries select `external_usage_reports`, not an additive total with inference attempts. `source/clients/cli/internal/ui/token_metrics.go` constructs the filter request and renders health, timeline, totals and breakdowns; `config_tabs.go` exposes Token Metrics. Cache/reasoning subset and non-billing-grade caveats agree with backend warnings. Corrected active-context navigation to `/c` or Settings Context, not `/context`. Price/baseline requirements are limitations, not a claim of implemented savings measurement. Verified by telemetry and focused CLI tests.

### 3. `advanced-routing.md`

`source/server/pkg/config/routing.go` defines task assignments as destination plus quality, named task defaults including Reconnaissance and Implementation, model quality mapping, and independent destination profiles. `ResolveDestination` (line 314) follows validated redirects without changing quality. `source/server/internal/inference/profilechain/chain.go` builds preferred/backup providers and classifies unavailable versus actionable credential failures; `route.go` also permits routes without backups. Thus Secondary is not automatically Primary's backup, and not every failure/route implies fallback. `source/server/internal/managedrouting/selection.go` enforces managed approved routes and local-only restrictions separately from developer preferences. Added cumulative delegated input-plus-output caps from `DispatchTokenBudget` (routing.go:111): Economy 1,000,000; Standard/Premium 10,000,000; distinguished the uncapped main conversation and monetary budgets. Tool grants remain a separate dispatch concern. Verified by config/inference tests and focused Routing UI tests; no provider failure was induced live.

### 4. `advanced-terminal-ui.md`

`source/clients/cli/internal/ui` implements rendered chat, layout/theme settings, context display, tool activity and child tabs; `config_tabs.go` and `config_surface.go` connect settings surfaces. Slash registrations in `internal/slash/{theme,context,contextview,export,basic,registry}.go` substantiate the listed commands. `/context` reads the project's `.cercano/context.md`; `/c` opens the conversation viewer. Added that distinction and trajectory-export entry, and corrected the launcher to `cercano-cli`. `ui/chat_tabs.go` (`closeSubAgentTab`) removes/persists dismissal without stopping the task; reopening restores child history. Acceptance/stop semantics are traced under delegation below. Interactive UI claims are not attributed to headless output. Verified by focused CLI tests, including context, theme, search, export and subagent cases; terminal appearance itself was not visually certified.

### 5. `automatic-session-retention.md`

`source/server/internal/conversation/store.go` persists turns in SQLite; host persistence implements listing/resume and structured turn restoration. `source/server/internal/recap/recap.go` best-effort generates titles; `SetGeneratedTitle` (store.go:950) protects user-renamed titles. CLI `slash/history.go` maps picker/resume/rename, and search is scoped to readable messages of the current conversation. `source/server/cmd/cercano/headless.go` resolves explicit `--conv` continuation separately from fresh default IDs.

Added destructive retention policy: config defaults are 90/180 days; `internal/retention/retention.go` disables aging for keep-forever or either non-positive horizon, otherwise collapses inactive conversations or prunes old raw bodies through the frozen boundary. `PruneRawBodies` (store.go:889) stubs content and clears structured bodies under both age/boundary predicates. `CollapseConversation` (line 906) deletes turns and compaction transactionally, retaining identity. Startup wiring in `cmd/cercano/main.go` starts/attaches the sweeper; config updates change subsequent sweeps. Slash config exposes the documented controls. Export is recommended before pruning, not represented as restoring already-pruned bodies. Verified by conversation, retention, recap and focused CLI tests.

### 6. `built-in-delegation.md`

`source/server/internal/capabilities/builtins/dispatch_cap.go` validates host-bound ownership, task/class/workdir and calls Tasks.Launch, returning task/child/status JSON. `builtins.go` maps `workflow` to dispatch. `source/server/internal/taskrun/manager.go:64` persists acceptance before starting independent execution, removes launching-context cancellation with `context.WithoutCancel`, and ties execution to host cancellation. Its run/Stop/Wait paths (lines 100, 169, 192) persist terminal states, stop execution explicitly, and let wait cancellation return without canceling execution. Same-parent completion can interrupt a wait. `conversation/tasks.go` reconciles unfinished tasks as interrupted on host startup, without replay.

`source/server/internal/hostsvc/tools/tools.go` (`GrantedRegistry`) defaults to PermR, resolves explicit grants, ignores unknown names and rejects an empty usable registry. Dispatch dynamic tier escalates non-read grants; `agent/permissions.go` supplies mode-dependent confirmation. Separate subagent context/tools do not sandbox the filesystem. `ui/chat_tabs.go` dismissal is not task stop. Qualified survival by host lifetime and last-client shutdown. Verified by taskrun/conversation suites, permission tests and focused child-tab tests.

### 7. `client-server-architecture.md`

`source/server/cmd/cercano/main.go` owns server startup; `pkg/agentclient/client.go` dials gRPC, tries an existing server first and auto-launches `cercano agent` when needed. The launcher marks it `CERCANO_AUTOLAUNCHED=1`. Main startup only enables idle shutdown when that marker and `agent.shutdown_on_last_client` are both set. Explicitly started servers are not marked for that lifecycle. `cmd/cercano/headless.go` uses the same client/server execution surface; server and terminal client have separate go.mod files. Host services own tools, providers, persistence and conversation-scoped state, not filesystem isolation. Corrected terminal launcher and distinguished persistent transcripts, independent task execution and host shutdown. Verification is source tracing plus task lifecycle/CLI tests, not a launched-server disconnect experiment.

### 8. `context-management.md`

`source/server/internal/hostsvc/persistence/persistence.go:558` assembles stored turns and schedules rather than awaits compaction. `internal/requestassembly/assembly.go` builds the send view, applies the in-memory elision floor, and, when enabled with a positive hard threshold/window, marks scheduling and applies superseded-output elision, recent-output retention, then oldest-message truncation; pairing is repaired afterward. This is not a synchronous summarizer call. `internal/compactiongen/compactiongen.go` debounces automatic work and implements incremental, clear and regenerate operations. Host `RegenerateContext` (persistence.go:1266) launches incremental work in the background but streams explicit regeneration progress. `internal/compaction/mapreduce.go` summarizes bounded older raw segments and deterministically merges summaries; this does not establish a user-selectable mode or that all rolling operations use it.

`ElideContext` (persistence.go:1169) changes an in-memory floor, not stored turns. Slash command mappings distinguish `/c` from `/context` and the compaction controls. Corrected that navigation, qualified automatic versus explicit operation, added the hard-limit preconditions and the inability to regenerate retention-pruned bodies. Verified by requestassembly/compactor/compaction/conversation and focused Context CLI tests.

### 9. `integrated-local-runtime.md`

`source/server/pkg/config/config.go` defaults `open_runtime` to `llama_server`. `cmd/cercano/main.go` builds/registers managed llama and mistral.rs providers; Ollama is a separate local inference backend, not a claim that the same managed provider installs Ollama. Runtime and Local Models tabs use the runtime dashboard; Routing controls placement separately. `source/server/internal/localruntime/manager.go:268` restarts instances and calls `refreshProviderConfig` (line 339); that function requires a loader and ConfigReloader, logs load failures and retains prior configuration. Qualified the previous unconditional reload statement accordingly. Runtime preparation is not a guarantee that all routes become local; resource/network caveats remain. Verified by focused Restart tests and Runtime UI tests. No binary/model installation, runtime restart or performance test was performed.

### 10. `openai-providers.md`

`source/server/internal/cloudfactory/factory.go` actually implements Chat Completions, Responses, messages and Bedrock despite a stale introductory source comment. Compatible endpoints receive configured URL/key/model; ChatGPT Responses and Claude messages subscriptions require distinct token sources. Subscription messages pins the provider endpoint rather than trusting a custom URL. Confirmed model evidence gates compatible Chat Completions/Responses vision support. Added that image-input qualification. Corrected profile editing to Settings Cloud and destination assignment to Routing: `source/clients/cli/internal/slash/config.go` shows `/cloud` lists, `/cloud use` selects, and `/cloud key` updates a key rather than opening the editor. `ui/config_surface.go` opens the actual Cloud editor. Verified by cloudfactory and focused Cloud CLI tests. Third-party compatibility, account entitlement, service availability, terms and pricing remain external uncertainties, not certified by this audit.

### 11. `powerful-agent-workflows.md`

`source/server/internal/protocols/catalog.go`, `protocols.go` and `capabilities/builtins/get_protocol.go` provide retrievable research/planning/execution/debugging/design/verification guidance, not enforced reasoning correctness. `builtins/web_research.go` validates query/setup, invokes the research pipeline and requests permission for durable output when applicable. Delegation is traced above. Added formal entry/handoff details: `suggest_plan.go` enters plan profile; `request_plan_approval.go` checkpoints the effort's spec/plan before returning to default, preserving plan mode on failure and warning rather than committing outside Git; `plan_exit` abandons planning. `autonomous.go` and `autonomous_execution_choice.go` persist the brief/run ledger and enter autonomous profile. `agent/permissions.go` preserves these human-handoff prompts in Bypass mode. Bounded direct execution versus formal planning is protocol guidance, not a mandatory planning prerequisite for every edit. Verified by protocol tests and focused Plan/Autonom/GateDecision tests. No research query or autonomous provider run was performed.

## Verification

All commands used `GOPROXY=off GOTOOLCHAIN=local`, with no dependency install, live provider request, service restart or commit.

From `source/server` (all 12 packages returned `ok`):

```sh
go test ./internal/taskrun ./internal/retention ./internal/requestassembly ./internal/compactor ./internal/compaction ./internal/telemetry ./internal/inference ./internal/cloudfactory ./pkg/config ./internal/protocols ./internal/conversation ./internal/recap -count=1
```

From `source/server` (all three packages returned `ok`):

```sh
go test ./internal/agent ./internal/capabilities/builtins ./internal/localruntime -run 'Test.*(Plan|Autonom|GateDecision|Restart)' -count=1
```

From `source/clients/cli` (both packages returned `ok`):

```sh
go test ./internal/slash ./internal/ui -run 'Test.*(Context|Routing|TokenMetrics|History|Resume|Search|Theme|Subagent|SubAgent|Runtime|Export|Cloud)' -count=1
```

Final documentation checks: `git diff --check -- docs/agent/features docs/audit-agent-features.md` passed. A local Markdown-link existence check found exactly 11 feature files and no broken local file targets (fragment anchors were not validated).

## Coverage boundary and uncertainty

Coverage is **11/11 guides**, including their feature descriptions, examples, controls, limitations and navigation. This is not a claim of exhaustive repository feature discovery, full test-suite coverage, billing accuracy, provider compatibility or deployed behavior. Focused tests exercise local/fake fixtures, not account entitlements or GPU/model readiness. Architectural and explanatory limitations (lossy summaries, shared filesystem, review requirements, prices/baselines) are deliberately not advertised as automatic guarantees. Existing unrelated worktree changes and historical plans were not audited or modified. The source's own uncertified inference-accounting coverage remains explicitly documented.
