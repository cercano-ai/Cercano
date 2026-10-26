# Isolation architecture — remaining source-audit closure

2026-10-23. Source comparison only; no live acceptance.

## Scope and disposition

Closed the remaining claim families for
`docs/agent/agent-isolation/architecture.md` listed in
`docs/audit-core-remaining.md:127–133`: provider construction, on-demand
credential/control proxies, runner execution/persistence wiring, and related
phase assertions. Read root `AGENTS.md` and the remaining checklist/completed
scope. This is a closure of those maintained documentation claims, not a
complete-file correctness proof or closure of the other guides' checklists.
No shared ledger was edited.

Earlier broker/pool/orphan, control receiver, host authorization, startup/default
and protocol-inventory comparisons remain bounded completed evidence. Batch 3's
codec/snapshot findings are carried forward, not repeated as whole-section
reviews: message extras and event metadata/kinds have documented omissions.
Nothing here upgrades those slices into end-to-end equivalence certification.

## Per-family evidence and corrections

Paths below are relative to `source/server/`.

### Provider construction — source comparison closed

- `internal/worker/worker.go:420–446,464–488,579–616,643–710`:
  dependencies rebuild from the turn snapshot; Primary and Secondary use shared
  `profilechain.Build`, not the former `wrapWorkerBackup` wrapper. Subscription
  routes receive ChatGPT/Anthropic stream token adapters; static routes fetch and
  validate credentials, while Bedrock skips that stream key fetch. Cloud
  providers are vision guarded. `llama_server` and `mistralrs` use host open
  inference; otherwise the worker creates an Ollama client only with a URL.
- `internal/inference/profilechain/chain.go:14–75` supplies destination/task-quality
  resolution, preferred/backup assembly and blocked/unavailable error handling.
  `internal/cloudfactory/static_credentials.go:14–55` distinguishes canonical
  vendor key requirements from anonymous/authenticating proxies; BaseURL alone
  is not a key exemption.
- `internal/worker/watchdog.go:16–77` and `worker.go:464–488,579–616`:
  watchdog is nil when disabled; enabled OneShot dispatch uses `TaskWatchdog`
  and the shared worker engine/task assignment. Removed the stale local-fast-model,
  `buildWorkerEngine` and `watchdogModelFor` description. Source construction
  is not evidence of live provider eligibility or successful inference.

### Credential and control proxies — source comparison closed

- `internal/worker/proxies.go:25–123,151–304,306–427`:
  serialized bounded sender; correlated permission, session-profile and credential
  responses; context-aware response waits and stale/unmatched response handling.
  The sender can block admission, and after a send failure it drains without
  promising delivery. Response cancellation is not a nonblocking-send guarantee.
  ChatGPT preserves account metadata; Anthropic's adapter discards it. Credential
  failure clears returned token/account.
- `internal/worker/host.go:833–860,992–1025,1054–1108`:
  credential resolution runs off the drain path; subscriptions use the shared
  host credential owner, static routes use secrets plus validation, and sanitized
  typed failures are returned. Session profile reads/switches invoke host callbacks
  and return success/error rather than mutating a worker-local session broker.
  Open-inference requests execute against the host provider.
- Corrected the claim that no secret sits in the child: no durable credential
  store is owned there, but fetched keys/tokens enter child memory. Static keys
  are used during provider construction; subscription adapters ask the host when
  the provider requests a token. Neither path proves successful refresh or live
  mid-turn static-key rotation. The four-operation list is now explicitly a
  subset, not a renewed protocol inventory. Prior control-receiver/authorization
  findings are not re-certified here.

### Runner execution and persistence — source comparison closed

- `internal/runner/core.go:195–244,267–276,459–503,508–569,644–774,795–864`:
  history assembly precedes persistence of current input; user input is forwarded
  before model execution when the callback and conversation ID exist. Subsequent
  assistant/tool-result messages use the callback. Whole-turn retry/fallback is
  blocked after visible text or tool execution; same-provider retry additionally
  requires a retryable error, and cancellation/live-context conditions apply.
  Execution ultimately calls `agent.RunToolLoop`. Host-Agent-dependent recap,
  compaction and context usage are not worker-local responsibilities.
- `internal/worker/worker.go:250–397,599–616` wires preloaded history and the
  stream persistence callback into the same core, with nil worker `Agent`;
  recovered panic becomes a terminal error, and sender closure precedes the
  terminal stream message. `internal/worker/proxies.go:429–445,479–607`:
  ordinary/sub-agent persistence and child creation are best-effort; marshal
  failure can drop a message. Dispatch-evidence append instead waits for host
  acknowledgment or cancellation. Preloaded history is the host-supplied value,
  not a fresh database read or target-aware rebuild in the child.
- `internal/worker/host.go:861–901` routes main-turn writes through the supplied
  host callback, sub-agent writes through their conversation ID without the
  main-turn fence, and acknowledges dispatch evidence after the store write.
  `internal/server/server.go:3699–3709` fences the main callback using the captured
  conversation/generation, not a new guarantee inferred from the wire `gen` field.
  `server.go:4082–4127` performs worker post-turn context accounting, recap,
  compaction and aggregate telemetry on the host.
- Corrected blanket persistence-fence, crash-durability and mode-equivalence
  wording. Enqueue, stream completion and ordinary one-way persistence are not
  durable-write acknowledgment; host post-turn usage is aggregate, unlike
  in-process per-model-call usage.

### Phase/status and invariants — reconciled

Sections 3–5 and 8–9 now distinguish shared core/front door from worker-specific
setup, transport, host callbacks and bookkeeping. The historical Done table is
not a current acceptance certificate. Removed current parity/safety conclusions
from old backup/watchdog implementation names and complete-codec assertions.
Host MCP execution and host-managed inference are explicitly outside the child
execution boundary; transport omissions prevent claiming identical event streams.
Historical parity/crash-test statements are labeled as not independently rerun.

## Focused offline checks and acceptance exclusions

Passed documentation diff/whitespace checks and local-link checks, with
cited-range and stale-claim spot checks. No Go build/test suite or race
suite was run; historical tests are not represented as executed by this audit.

No provider requests, OAuth login/refresh/rotation, credential-store mutation,
network access, installs, server restart, process spawn/SIGKILL, OOM, orphan
reaping, concurrent live conversations, passive attach, permission UI, MCP tool
execution, database durability/crash recovery, or end-to-end event-stream parity
was exercised. Provider rebuilding is compared at the per-turn worker boundary,
not every host settings-invalidation caller. Evidence acquisition, full cloud
routing resilience and other guides remain outside this delegated closure.
Source audit completion does not assert operational acceptance of isolation or
security sandboxing of arbitrary tools/host callbacks.

## Integration readback — 2026-10-23

Narrow source spot-checks confirmed provider key fetch/validation and shared
Primary/Secondary chains (`internal/worker/worker.go:643–710`), best-effort
one-way persistence (`internal/worker/proxies.go:429–445`), main/sub-conversation
routing and dispatch acknowledgment (`internal/worker/host.go:861–901`), the
captured-generation callback (`internal/server/server.go:3699–3709`), and
watchdog task dispatch (`internal/worker/watchdog.go:16–77`). This readback did
not repeat the audit. Qualified two residual architecture invariants: sub-agent
writes bypass the main-turn fence, and cleanup is not a no-orphans guarantee.
The four shared ledgers now link this report and supersede isolation-pending
status only; README/cloud-routing closure is left to its separate work.

Integration checks passed: scoped `git diff --check`, full-file trailing-whitespace
checks, and existence checks for 75 relative Markdown link targets across the
architecture, this report and the four shared ledgers. External URLs and fragment
anchors were not checked. No application tests or live acceptance were run.
