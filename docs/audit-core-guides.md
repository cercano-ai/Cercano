# Core guides semantic audit

## Latest disposition — 2026-10-22

The local closure pass below completes the decision-fidelity maintained-claim
comparison, but **does not complete the requested five-guide coverage**. README,
self-development, cloud routing and isolation architecture remain partial.
Publication of these corrections must not be read as closure of those gaps.

## Earlier disposition and boundary

**Partial — this does not close complete-file semantic coverage for the five guides.**
Scope: `docs/agent/README.md`, `docs/agent/self-dev.md`, `docs/cloud-routing.md`, `docs/agent/agent-isolation/architecture.md`, and `docs/agent/compaction-decision-fidelity.md`, on the supplied `a133f3359` baseline. Historical incident accounts and phase milestones are not current acceptance claims. No live product acceptance, provider inference, installation, restart, or commit was performed. The shared audit ledger was not edited.

The following distinguishes explicit source comparison from remaining coverage. Existing working-tree corrections are not, merely by being present, evidence that every surrounding claim was verified.

## Per-file evidence

### `docs/agent/README.md` — partial

Explicit downstream comparison confirms the corrected direct-tool description at lines 143 and 329:

- `source/clients/cli/internal/ui/model.go:3378–3395`: cached R and unknown permissions invoke directly; known non-read permissions queue a local confirmation. This branch does not consult bypass mode.
- `source/clients/cli/internal/ui/model.go:5017–5044`: local approval calls `invokeToolCmd`; local denial does not execute. Stream-origin approvals and denials instead use `AllowToolCall` and `DenyToolCall`.
- `source/clients/cli/internal/ui/model.go:5056–5085`: compose redirection is restricted to stream-origin ordinary tools. MCP “always” sends the persistence RPC only for stream-origin calls; the local callback dismisses the prompt without execution or persistence.
- `source/server/internal/server/server.go:2385–2423`: unary `InvokeTool` resolves the registry entry and calls `Execute` directly, with no tool-loop permission gate. Errors are returned in the response. This is not a server-enforced approval boundary; individual tools can still validate/reject their input.

Remaining complete-file coverage includes setup/launcher and headless behavior, the other slash-command downstream handlers, model-driven permission exceptions, provider resilience/authentication, autonomous completion, attachment lifetime, persistence/resume, and session model selection. The direct-tool comparison alone does not certify these sections.

### `docs/agent/self-dev.md` — partial

No complete-file implementation comparison is certified by this report. Remaining maintained claims include dev-mode admission/resume/rollover metadata, delegation/task lifecycle and routing, build/signing/launcher behavior, storage and log paths, timeout/cancellation, tracing switches, current compaction/recap routing, and reconnect behavior. Dated troubleshooting anecdotes remain historical and are not a live acceptance requirement. The documented opt-in live-model test was not run.

### `docs/cloud-routing.md` — partial

No complete-file implementation comparison is certified by this report. Remaining claims include default assignments, redirect validation and final-destination policy, account traversal and quality resolution, image selection/cache behavior, identity acquisition/storage, settings draft isolation and failure handling, wizard rollback, and per-turn worker configuration. Existing statements about earlier synthetic/loopback/CLI verification are not new test results from this pass. Provider availability, eligibility, quality, pricing, and retention are outside the offline source-verification boundary, not prerequisites for verifying repository behavior.

### `docs/agent/agent-isolation/architecture.md` — partial

Corrected the protocol sketch at lines 334–337 to label it a **core lifecycle subset**, rather than an exhaustive oneof inventory. `source/proto/agent.proto:1792–1845` includes additional open-inference, session-profile/model, authentication, runtime-restart, MCP, permission-update, autonomy-ledger, dispatch-event, model-authorization, accounting, and task-control messages.

Additional implementation pointers inspected in this pass:

- `source/server/internal/worker/worker.go:236`: applies runtime permission updates; `:548` registers advertised MCP proxies.
- `source/server/internal/worker/host.go:762–778`: receives model-authorization requests and request-accounting messages.

These symbol/call-site checks establish relevant protocol coverage but do not alone verify complete end-to-end semantics. Remaining coverage includes the complete runner/service inventories, host/worker lifecycle and pool cleanup, credential and control proxies, broker replay/overflow and persistence fences, crash status propagation, startup orphan cleanup, and execution-mode defaults. Existing lifecycle/MCP corrections must not be mistaken for complete-file certification. Historical phase bodies need not be rewritten as current implementation claims.

### `docs/agent/compaction-decision-fidelity.md` — partial; focused offline checks pass

Located production entry points in `source/server/internal/compaction/summarizer.go:29–32` (shared prompt), `:78` (parser), and `:187` (evidence renderer). `source/server/internal/loopcompact/wiring.go:165` calls the task-aware prompt; `:218` defines the per-dispatch factory, installed by `source/server/internal/worker/worker_dispatch.go:100`.

The four focused deterministic fidelity tests passed in this pass. This verifies their exercised assertions, not live-model classification accuracy. Complete-file source certification still requires closing the remaining prompt/parser/rendering, budget, merge/retention, and main-thread/dispatch wiring comparison; grep locations alone are not full function verification. The existing parser/task-reference addition must be evaluated with that boundary in mind.

## Offline checks performed

From `source/server`, with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`:

```sh
go test ./internal/compaction -run 'TestSummary(DecisionFidelityContract|TranscriptPreservesToolEvidence|SourceReferencesStableAndEvidenceSensitive|FidelityAcrossRepeatedCompactions)$' -count=1
```

Result: **PASS**, `ok cercano/source/server/internal/compaction 0.329s`. No dependency installation or live-model request was required.

`git diff --check`: **PASS** after the protocol-sketch correction. No broad test sweep was performed. No manual terminal or live/end-to-end acceptance is claimed.

## Publication review follow-up

Coverage remains **PARTIAL** for all five files. Publication review read the
actual diff and spot-checked CLI local confirmation and unary execution,
model-driven denial/preauthorization/watchdog gates (`agent/toolloop.go:1017–1135`),
task-reference encoding and lenient parsing (`compaction/summarizer.go:32–120`),
worker runner selection (`server/server.go:1532–1537`), and pool release / receive-error
handling (`worker/pool.go:178–199`, `worker/host.go:730–758`). Source paths are under
`source/server/internal/` except the CLI evidence above. Clarified that the host
currently accepts any receive error after `TurnDone` as completion, not only EOF.
These checks do not certify surrounding sections or complete-file semantics.
The cumulative ledgers now link this explicit partial disposition.

Publication checks: scoped `git diff --check` passed (including a separate
untracked-report whitespace check); 62 relative inline Markdown targets across
the eight allowed paths exist. Fenced code, images, reference-style links,
anchors and external URLs were excluded. The focused Go tests above were not
rerun during publication review; no runtime acceptance is claimed.

## Local closure pass — 2026-10-22 (baseline `729bb6d0`)

The preceding partial reports are historical records; dispositions below supersede
them only for files explicitly completed. Source verification is not runtime acceptance.

### Decision fidelity — completed maintained-claim comparison

Read the guide once and compared its contract, references, parser, tests, budget,
merge/retention and shared wiring. `compaction/summarizer.go:28–209` implements
the status contract, JSON task reference, lenient parser and content hash; corrected
the guide to exclude messages with no rendered evidence.
`compaction/budget.go:35–127` counts the full rendered prompt and output reserve;
`budgeted_summarizer.go:22–62` elides then chunks and deterministically merges;
`dedup.go:18–164` preserves IDs while stubbing superseded/old/oversized results.
`summary.go:15–113` uses lexical dedup, later file states and fixed rendering, not
semantic conflict resolution. `loopcompact/wiring.go:72–170,218–258`,
`cmd/cercano/main.go:340–384` and `worker/worker_dispatch.go:95–103` connect the
same prompt to main and dispatch paths. Paths above are under
`source/server/internal/` except `cmd/`. Read all four deterministic fidelity
tests (`decision_fidelity_test.go:14–192`); their scripted outputs cannot certify
model classification or repair stored summaries. No live model or restart tested.

### Isolation architecture — additional maintained-claim comparison

Compared runner request/dependency inventories (`runner/runner.go:45–140`),
worker permission/control receivers (`worker/worker.go:180–260`), receive-error
and authorization handling (`worker/host.go:725–820`), pool acquire/release,
health and idle reaping (`worker/pool.go:92–388`), orphan decisions
(`worker/reap.go:65–140`), and broker generation/replay/subscription behavior
(`broker/broker.go:100–310`). Source paths are under `source/server/internal/`.
Corrected an important boundary: normal release neither increments generation
nor closes passive subscriptions; supersession/cancellation fences stale
generations. Qualified phase-era inventories and historical equivalence claims,
and documented unwired permission-update sending and best-effort orphan cleanup.
Config defaults/startup call sites remain `pkg/config/config.go:35–43,639–643`
and `cmd/cercano/main.go:400,645`. This extension does not establish complete
codec, provider, credential-proxy or runner-core semantic coverage; architecture
coverage remains partial. No spawned-worker crash or OS cleanup acceptance run.

### Cloud routing — additional identity comparison, still partial

`source/server/internal/anthropicauth/identity.go:15–66` confirms advisory
sign-in identity lookup, a two-second timeout, refusal to follow redirects,
a 64-KiB JSON read bound, and preservation of existing identity fields. A custom
token endpoint does not implicitly send its bearer token to the default profile
host. This is not verification of account identity storage, other providers,
image caching, wizard rollback or live profile responses. The earlier remaining
coverage list is not closed by this comparison.

### Publication checks for this increment

Offline focused tests passed with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`:

```sh
go test ./internal/compaction ./internal/broker -run 'TestSummary(DecisionFidelityContract|TranscriptPreservesToolEvidence|SourceReferencesStableAndEvidenceSensitive|FidelityAcrossRepeatedCompactions)$|Test.*(Release|Supersed|Attach|Cancel|Fence)' -count=1
```

Results: compaction `0.291s`, broker `0.575s`. These are deterministic checks,
not live-model classification, UI/reconnect acceptance, process-crash cleanup,
provider eligibility/availability/quality/pricing/retention or stored-summary
repair certification. No installations, service restarts or inference calls.
README and self-development have no new completed disposition in this increment;
the explicit remaining local claim families above still apply. The worktree is
retained for those gaps and review, not removed after publication.

Publication target check: 8 scoped Markdown files, 73 relative inline/reference
filesystem targets, zero missing. Fenced code, anchors and external URLs were
excluded. `git diff --check` passed for this documentation increment.
