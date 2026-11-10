# Recovery cleanup — Phase 4

## Before restructuring: baseline and policy

Baseline `d1f668f5200e639f7efecb3b9c1e287c001ebb35`; working tree clean. Read AGENTS.md, CLAUDE.md, approved plan/spec and Phase 3 evidence. Spec proposed-status language is historical under explicit execution authorization. No nested source guidance found.

Baseline ordinary outcomes: ready acquire (optional journal/receipt operation identity), guarded dead-owner recovery, durable journal retry, exact local/reserved claim, modern force relinquish, old-host cooperative relinquish, or refusal at each authentication, ownership, persistence and transport boundary. The deferred fallback runs after virtually every non-ready ordinary return, except cancellation and the ready recovery-receipt error suppression. Consequently unrelated refusals can be replaced with an offer or discovery error, hiding the original stage.

Proposed narrow policy, recorded before production edits:

| Ordinary outcome | Containment discovery eligibility |
|---|---|
| Ready acquisition / exact local or reservation claim / successful cooperation | None; preserve readiness and exact operation identity |
| Missing destination routing (empty endpoint only), absent cooperative resolver/dial capability, foreign historical reservation without a matching journal | Eligible discovery; exact source/selection/adapter qualification and fresh consent still required |
| Source resolver reports not found or unavailable; pre-mutation dial/list reports unavailable | Eligible discovery only; unreachability is not death or stop authority |
| Explicit old-host cooperative FailedPrecondition refusal, after rereading exact unchanged owned row | Eligible discovery only |
| Authentication/identity mismatch, cancellation/deadline, invalid routing, unknown resolver errors | Refuse, no discovery |
| Journal malformed/mismatched/unreadable; ownership or receipt lookup error (other than explicit absence) | Refuse, no discovery |
| Acceptance, cleanup, publication, local restore/claim, guarded recovery failure | Refuse, no discovery; retain uncertain state |
| Mutating source RPC response loss / unknown failure / receipt mismatch | Refuse; legacy lost response may still claim an exactly proven reservation, never infer stop authority |

Original ordinary refusal stage/check/code/classification must be logged before eligible discovery, using existing redacted diagnostics, never raw errors, credentials or receipt contents. Discovery diagnostics are distinct. No wire or adapter changes planned.

## Resumed verification: blocked, not a completed Phase 4

Re-read repository guidance, approved execution task, plan/spec and Phase 3 evidence. On this resumed run HEAD remained `d1f668f5200e639f7efecb3b9c1e287c001ebb35`; the tree was **not clean**: plan, ownership.go and resume.go were modified and this record and resume_policy_test.go were untracked. The clean baseline assertion above describes the earlier characterization, not this resumed inspection. Earlier reported red/green results in conversation history are not independently rerun baseline evidence here.

Added isolated `receipt-lookup` and `guard-error` probes before attempting further production correction. Both reproduced forbidden error-to-offer conversion on the working candidate: `ready=false confirmation=true code=OK discoveries=1`. Fixtures use temporary SQLite stores and loopback RPCs only. Receipt probe drops only its disposable fixture table. Assertions also check unchanged durable ownership and no legacy/contained stop calls. Failure messages now report booleans/code/count, not the response containing synthetic receipt/selection material.

Exact commands executed from `source/server`:

```
go test ./internal/server -run '^TestResumeFallbackPolicy/(guard-error|receipt-lookup)$' -count=1 -timeout=120s
go test ./internal/server ./internal/conversation -run 'Test(DestinationResume|Resume|LiveSourceRecovery|LiveRecovery|ExactRecovery|RecoveryUncertain|Ownership|Handover)' -count=1 -timeout=180s
go test -race ./internal/server ./internal/conversation -run 'Test(DestinationResume|Resume|LiveSourceRecovery|LiveRecovery|ExactRecovery|RecoveryUncertain|Ownership|Handover)' -count=1 -timeout=180s
```

All three exited 1. Server failures in each were the two named new probes. Both broader commands passed the conversation package; race execution reported no race, but this is **not** a passing race gate. `git diff --check` passed. No full-suite, compilation-only, native-platform, VM or historical production acceptance claim is made.

### Escalation requiring policy/interface resolution

`resume.go` guarded-recovery inspection discards its error and later permits eligible discovery. `dead_owner.go` converts every RecoveryGuard error into FailedPrecondition. The production guard (`cmd/cercano/development_recovery.go`) returns untyped errors for metadata read/identity/duplicate-record failures as well as absence-verification/live-source failures. Thus the current server cannot reliably distinguish ordinary live-source refusal from genuine guard metadata failure. Ignoring all guard errors violates the proposed fail-closed policy; making all errors terminal before cooperation would interfere with cooperative-first behavior. Choosing a typed guard outcome or restructuring guard ordering/eligibility across this boundary is left unresolved rather than silently changing the approved contract or doing Phase 5 adapter work. No new wire representation is needed or authorized.

Separately, ignored HandoverResult errors in the ordinary path still permit discovery (the receipt-lookup probe). This straightforward correction remains unapplied while the candidate is blocked. Diagnostic redaction/context coverage, dedicated acceptance-cleanup failure coverage, complete uncertainty coverage and readiness/history review are not claimed complete.

### Checkpoint and limitations

Checkpoint only this evidence, semantic plan statuses and the focused test file as a **red characterization/blocker checkpoint**, not a release-ready implementation. The existing production candidate changes in `internal/server/resume.go` and `internal/conversation/ownership.go` remain uncommitted and preserved for review; they must not be treated as verified. The policy table above is proposed eligibility, not proof that the current candidate enforces it. Phase 4 is incomplete. No live hosts, user data, VM, merge, push, schema/generated wire edits, or evidence deletion occurred in this resumed run. No Phase 5/6 task was executed.

## Phase 4 completion — 2026-11-10

The blocked record above is retained as historical evidence. Explicit user authorization resolves its typed-guard design blocker. Preserved and reconciled the existing resume/ownership candidate; no wire, schema, consent or stop-authority changes. Actual producers classify only exact live-process detection and launch-lock contention as `recoveryguard.ErrLiveSource`. Metadata, identity, inspection and unknown errors retain their causes internally and fail closed for discovery, without preempting authenticated cooperation or exact claims. RPC guard diagnostics are generic; ordinary stage/check/code/classification is logged before discovery without raw credentials, receipts, endpoints or store paths.

Reproduced both red probes using a temporary Go overlay of `resume.go` from checkpoint `68d2301cb4ffbc295206120a76c1b68a79550df0`, preserving the candidate on disk. Command: `go test -overlay <temporary-overlay.json> ./internal/server -run '^TestResumeFallbackPolicy$/(receipt-lookup|guard-error)$' -count=1`. Exit 1: both returned `ready=false confirmation=true code=OK discoveries=1` and reported missing original diagnostics. This is checkpoint-source reproduction, not a claim to have reverted or freshly reproduced the original dirty candidate before its inherited edits.

Current candidate verification (all exit 0):

- From `source/server`: `go test ./internal/server ./internal/conversation ./internal/brewrestart ./cmd/cercano ./pkg/agentclient -run 'Test.*(Resume|Recovery|Recover|Ownership|Handover|DevelopmentRecovery|VerifyHostAbsent)' -count=1 -timeout=180s`.
- Same command with `-race`: all five packages passed, no race reports.
- From `source/clients/cli`: `go test ./internal/ui -run 'Test.*(OriginalRecovery|ForcedResume|Recovery)' -count=1 -timeout=180s`; same with `-race`: passed.
- `go test ./... -run '^$'` in both modules: passed compilation, not full-suite execution.
- `git diff --check`: passed.

Coverage includes authentication/identity mismatch, cancellation/deadline, unknown resolver/guard errors, receipt/journal lookup errors, malformed journals, definitive publication and acceptance-cleanup failure, uncertain publication, lost source responses, missing/unreachable sources, typed live-source refusal, old-host cooperative refusal and success, adapter-less cooperation and exact claims. Ready journal checks and recovery retry fixtures distinguish current ready ownership from historical receipt identity. Eligible discovery records the original refusal; qualified adapters do not make terminal errors eligible. Zero-stop assertions cover ordinary failures and successful cooperative/exact-claim paths. Darwin guard producer tests use injected process inspection, not live-host recovery.

Limitations: focused fixture coverage, not full test suites, historical production acceptance, cross-platform runtime qualification, VM or live deployment. No live hosts/user state/VM/push/merge accessed; no Phase 5 adapter consolidation or Phase 6 acceptance performed. Earlier blocked evidence is superseded only for this Phase 4 candidate and these checks.
