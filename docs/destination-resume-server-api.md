# Destination resume contract and implementation

`LifecycleResume(LifecycleControlRequest)` runs on the **already launcher-selected
destination**. Normal CLI resume uses that destination's existing authenticated
control connection. It does not select/start another destination, duplicate launcher
build logic, or attach to the historical source. `defaultSelection` is unchanged.

## Caller and retry contract

Supply the exact selected `target`, `owner.conversation`, registered loopback
`destination_endpoint`, and current requester `LifecycleIdentity`. Do not supply a
source endpoint/token. The server resolves the exact owning source from private
host records and authenticates its lifecycle identity.

A successful `ready=true` response includes current destination ownership. Capable
live sources force-relinquish automatically; verified dead owners recover
automatically without routine confirmation. Force `Unimplemented` falls back to
cooperative relinquish. Neither RPC unavailability nor a generic failure proves
death: only the trusted recovery guard does, with repeated exact-epoch checks.

Transport retries use **the same destination and conversation**; do not manufacture
a new handover ID. The private store-adjacent `.resume-operations` journal retains
source, owner epoch, operation ID and receipt before source mutation. Interrupted
reservation/claim retries survive destination/UI restart and lost responses.
Historical claims cannot restore changed epochs.

## Blocked legacy source

Blocked cooperative relinquish returns `confirmation_required=true`, `ready=false`,
exact `source`, observed `owner`, stable `operation_id`, diagnostic `blocked_reason`,
and, only when a verified adapter can prepare one, `legacy_process_identity`.
This response authorizes **no stop or signal**.

The CLI asks explicit whole-source-host interruption confirmation, names the source
and requested conversation, and warns that sibling conversation/task inventory is
incomplete and external effects may remain uncertain. Decline and stale UI callbacks
send no continuation. Acceptance repeats `LifecycleResume` on the same destination
with `confirm_legacy_stop=true`, the offered source/owner/operation/process identity,
and original requester. Server validation binds these fields to the durable journal;
a changed requester, source, epoch, operation or process identity fails closed.
Destination and requester hosts cannot be stopped.

`LegacyProcessStop` is an injected trusted platform boundary: preparation must prove
UID, PID/start identity, executable and exact host attribution. Stop must hold source
launch exclusion, revalidate the snapshot and operation immediately before signaling,
and verify death. Recovery independently repeats the established absence guard before
exact-epoch recovery. A lost stop response can finish through guarded recovery without
a second stop; approval is not inferred from a prior confirmation-required response.

### Production source-only shutdown

Darwin/cgo production wiring now supplies `developmentLegacyStop`. Preparation binds
one exact private host record and its `.starting` generation to the source lifecycle
identity, generation executable, same-user kernel PID/start identity, exact host
argument and loopback listener. Preparation and stop use the existing per-generation
launch exclusion. Stop rechecks the approved record and process, then delegates to
the launcher's existing single-target TERM/wait/KILL/verified-exit helper with durable
approval/epoch revalidation immediately before signal authority. It never invokes
upgrade-wide `StopUpgradeHost`, enumerates additional stop targets, or changes latest
host selection. Shutdown has a ten-second deadline plus the helper's existing bounded
cancellation cleanup. Ownership recovery independently verifies absence under launch
exclusion before changing the exact owner epoch.

Missing, ambiguous or changed source records/processes and unsupported platforms
fail closed. Destination and requester hosts are excluded. An offer without a verified
process snapshot produces no actionable CLI confirmation. Repeated offers replace
stale process metadata; the old approval cannot authorize the replacement. Whole-host
impact remains explicit: sibling inventory is incomplete, so all source conversations
and running tasks may be interrupted, not merely the requested conversation.

## Production adapter verification (2026-11-03)

The stale-offer and CLI no-adapter regressions were reproduced before their fixes.
Production-adapter fixtures exercise private source records, generation binding,
launch exclusion, source-only shutdown, changed process/record/generation, decline,
and rejected approval without controlling live hosts. Existing disposable launcher
workers cover TERM cooperation, KILL escalation, verified exit and cancellation;
server fixtures separately reject ownership recovery while the source remains alive.

Final checks passed:

- Server: `go test ./... -count=1 -skip '^(TestDevHostTerminalPTY|TestEverydayLauncherEntrypointRefusalsPTY)$'`
  (includes integration tests; only the two established baseline failures below excluded).
- Server: `go test -race ./internal/server ./internal/brewrestart ./cmd/cercano -run 'Test.*(DestinationResume|Legacy|DevelopmentRecovery|UpgradeHostTERMPolicy|UpgradeHostCancellation)' -count=1`.
- CLI: `go test ./... -count=1` and `go test -race ./internal/ui -run 'Test.*(Legacy|DestinationResume|ForcedResume)' -count=1`.
- Server and CLI entrypoint builds to `/tmp/cercano-resume-legacy-{server,cli}-20261103c`.
- `git diff --check`.

No live hosts were stopped and nothing was deployed. Unsupported platforms remain
fail-closed; Darwin/cgo is the production stop implementation.

## Earlier verification (2026-11-03)

Only isolated loopback test peers, temporary stores and injected process adapters
were used; no live host stop, production mutation, deploy or restart was performed.
Targeted tests were observed failing before wiring/fixes (CLI old acquire/recover RPC
policy, then stale-requester approval); regression coverage includes selected CLI
destination, automatic dead recovery, capable force, cooperative fallback, legacy
accept/decline/stale identity, sibling-impact wording, restart and lost stop response.
A new unsupported-at-offer test first failed with a misleading stale-approval error;
validation now explicitly reports safe legacy process stop unavailable when no
verified process snapshot could be offered, without signaling. The scoped checks
below were rerun after that fix.

From `source/server`:

- `go test ./internal/server ./internal/conversation ./pkg/proto -count=1` — passed.
- `go test ./internal/server -run '^TestDestinationResume' -count=1 -race` — passed.
- `go test ./cmd/cercano -run '^TestDevelopment' -count=1` — passed.
- `go build -o /tmp/cercano-resume-contract-diag-20261103c ./cmd/cercano/` — passed.

From `source/clients/cli`:

- `go test ./... -count=1` — passed (including UI).
- `go test ./internal/ui -run 'Test(Resume|LegacyResume|DeadOwner|LauncherResume|ForcedResume)' -count=1 -race` — passed.
- `go build -o /tmp/cercano-cli-resume-contract-diag-20261103c .` — passed.
  An initial build invocation incorrectly used nonexistent `./cmd/cercano/`; corrected
  to the CLI root entrypoint above.

### Command-package failures: baseline vs regression

The earlier comparison used `9418eea799d9947282ab9e91814cf8d9efef6dbb`.
That commit **already includes the first server resume changes** and was not a
valid pre-feature baseline. Its actual parent is
`a4d5604933e61091bad246c68c036bdcbcaf3295`.

Reran from `source/server` on the repaired working tree and a fresh `git archive`
of that parent (no git worktree registration or switching):

```
go test ./cmd/cercano -run '^(TestDevHostTerminalPTY|TestEverydayLauncherEntrypointRefusalsPTY)$' -count=1 -v
```

Verified all 3,353 archived tracked blobs against the parent tree (zero mismatches).
**Both tests fail with the same failure signatures on the actual pre-feature
parent and repaired tree**:

- `TestDevHostTerminalPTY`: `dev_host_terminal_test.go:401`, PTY ended before
  release, exit 1, missing durable fixture history after reconnect; context-meter
  warnings include `coexistence write requires explicit ownership capability`.
- `TestEverydayLauncherEntrypointRefusalsPTY`:
  `everyday_launcher_acceptance_test.go:55`, exit 78, `config authority must be
  canonical and inside isolated prepared state`, before the expected injected
  publication failure.

Thus these two observed failures predate the first server feature commit; the
comparison with `9418eea` alone did not establish this. They remain unresolved;
the command package/full server suite is not green and no full-suite sign-off is
claimed. Logs: `/tmp/resume-a4d5604-pty-base.log` and
`/tmp/resume-review-pty-current.log`. The pre-feature archive is retained at
`.resume-review-base/` (untracked). The older `.resume-baseline/` and its logs are
historical comparison artifacts, not the pre-feature baseline.

### Review fixes: publication and guarded-recovery retirement

Both retirement defects were reproduced before fixes (see
`/tmp/resume-review-red.log`), and independently rerun against an isolated archive
of original combined HEAD `161b3bbed2db028d2ac5b54898c8d5ba97cb7581`
(`/tmp/resume-review-before.log`, archive `.resume-review-before/`).
Definitive publication failure left one unjournaled participation; retry and release
still failed retirement. Blocked legacy confirmation followed by source death and
guarded recovery likewise left participation blocking retirement in-process.
The restart variant was already passing and remains covered.

The destination now removes only the exact unpublished acceptance when journal
publication definitively fails before rename. After rename, directory-sync/open
failure is treated as uncertain: retain receipt and operation identity, and
reaffirm journal durability on retry before any source mutation. Successful
guarded exact-epoch recovery reconciles only the matching journal participation;
other handovers, even for that same conversation/source epoch, remain fenced.
No production legacy stop adapter was changed.

New tests cover both original failures through retirement after release,
uncertain publication with repeated failing sync and zero source calls,
same-process/restart identity-preserving retry, recovery across restart, and
preservation of unrelated participation. Scoped checks passed:

```
go test ./internal/server -run '^TestDestinationResume' -count=1 -v
go test ./internal/server ./internal/broker ./internal/conversation ./pkg/proto -count=1
go test -race ./internal/server ./internal/broker ./internal/conversation -count=1
```

Logs: `/tmp/resume-review-regressions.log`, `/tmp/resume-review-scoped.log`,
`/tmp/resume-review-race.log`. An initial scoped invocation included nonexistent
`./internal/devhost`; it was corrected to the package list above.
These runs use isolated test fixtures only; no live-service RPC, production store,
deployment, landing, or push was performed.

### Cooperative takeover: fresh consent for an exact pending operation

A timeout consumes the confirmation challenge, not the retained operation. After
real completion tickets finish (or persistence uncertainty is explicitly
reconciled and the tickets completed), refresh Resume and explicitly confirm the
new challenge. The coordinator reuses the **same** authenticated operation,
source epoch, destination and receipt; it does not replace the source's pinned
request. This also covers a lost successful relinquish response and a lost
coordinator response after destination claim.

Store-adjacent `.takeover-operations` journals are discovery hints only. Retry
requires the live authenticated source's exact retained request/result and the
destination's retained acceptance. Publication durability is reaffirmed before
source mutation. Ready additionally requires the claim's ready local capability
and current durable owner epoch; historical claim receipts cannot restore an
obsolete operation. Challenges remain requester-bound, expiring and single-use.
No unsupported takeover RPC falls back to an older mutation RPC.

Limitations: this is not crash reconstruction of authority. Missing journals,
lost in-memory source/destination acceptance, changed destination endpoints,
uncertain source commit/cleanup, and journal publication without a retained
source request can still require diagnosis rather than an automatic retry.
There is no automatic replay of interrupted tasks or external effects and no
rollback of external effects. Claim reloads durable conversation history through
the existing prepared reload path, which **does schedule durable completion
notifications**; reload must not be described as starting no work. The response
loss tests cover source RPC loss and coordinator response loss after the local
claim (the coordinator calls claim locally, not over a separate claim RPC).
