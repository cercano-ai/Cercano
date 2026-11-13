# Bounded resume observation

## Review correction

Ordinary preflight now uses the trusted private local host record for the Store
binding and freshly authenticated host/build/contract/schema identity. It does
**not** call `LifecycleList`. List observes launch receipts and can mutate them;
a synchronous client RPC deadline does not join the remote handler. A canceled
List may later acquire the lifecycle mutex and consume a receipt. Client observer
capacity does not bound remote handlers left queued by such calls.

Normal resolution and safe authentication share an eight-slot lifecycle-wide
observation pool and five-second preflight budget. Expired callbacks keep their
slots until completion; late connections are closed by their sole result owner.
Recovery has a separate bounded eight-slot pool for independent saved-record
resolution and selection, so exhausted ordinary capacity cannot starve it.
Neither pool promises to bound arbitrary remote work.

`GetDevelopmentHostIdentity` reads immutable construction options without the
lifecycle mutex. Retired state is read through admission's own synchronized
snapshot. Thus newly built hosts can answer identity even while List is queued
on the lifecycle mutex. List's existing receipt semantics are unchanged.

## Historical limitation (fail closed)

Old running binaries cannot be repaired retroactively. Historical identity
handlers take the same mutex as List. If an old host is stuck holding that mutex,
fresh identity authentication cannot succeed: **confirmed live-source recovery
is impossible for that stuck old build**. Recovery must refuse; saved records
alone never authorize termination. Upgrading the destination does not fix this.
The legacy full-identity discovery helper still uses List for compatibility;
it is not part of the abandonable ordinary resume observation path and cannot
claim remote cancellation/join guarantees.

Prepare, source stop, launch exclusion, journal and ownership operations remain
foreground. Exact process identity, fresh authentication and consent, epoch
fences and writer absence remain mandatory. No claim is made that returning
from a client RPC joins a remote mutation. Arbitrary context-ignoring mutating
adapters are not made bounded by this observation mechanism.

## Historical disposable verification

Reproduction against the original identity implementation: both
`TestIdentityIndependentOfLifecycleMutex` and `TestIdentityWhileActualListQueued`
fail because production identity queues behind the lifecycle mutex. With the
fix both pass; the latter also demonstrates that an actual canceled List can
still consume its launch receipt after mutex release.

`TestDevelopmentResumeTimeoutConfirmedContinuation` exercises production resume
handlers, real private records, token-authenticated identity, and explicit resume
confirmation. It injects an immediate `DeadlineExceeded` and substitutes stop,
process inspection, absence, and destination seams; no live process is signalled.
It does not exercise an actually expired production observation through native
stop in one flow. Timeout, ignored-callback, actual identity-lock, and recovery
capacity-exhaustion behavior have separate tests. Fresh identity and consent are
not replaced by fixture shortcuts in the adapter authentication path.

Focused race tests (server and command packages) and all integration race tests
passed. The first full-suite run exposed a stale test that injected source auth
failure through List; it now injects refusal at authenticated ControlDial,
matching the production boundary.

Final verification used `/opt/homebrew/bin/go` (Go 1.26.3 darwin/arm64):

- Focused `-race` server/command tests, including destination resume and the new
  production-adapter continuation: PASS.
- `go test -race ./test/integration -count=1 -timeout 120s`: PASS.
- `go test -p 1 ./... -count=1 -timeout 180s`: PASS (all server packages).
- Production command build to a disposable `/tmp` output: PASS.
- Default-parallel full-suite runs had an unclassified parallel-suite failure in
  `TestRemoteRecoveryLogsNewerHostOnce` (`launches=1`, expected zero). That test
  passed three isolated repetitions and no modelhost code was changed, but neither
  fact establishes that the failure is unrelated or preexisting. An initial
  serial run hit the tool's five-minute ceiling; the final serial run completed
  successfully without that ceiling. Serial success does not replace the
  default-parallel landing gate.

## Landed audit fixes and historical verification — 2026-11-13

Baseline bounded identity-only observation landed in local main `f888165d`.
The remaining audit diagnostics and retry fixes landed in local main at
`a08f94f1f7afb66d866588ea86eea6e25c05905f`.
They add correlated, safe asynchronous terminal release failure diagnostics without
clearing unresolved blockers/ownership, and retry final prepared-writer absence
only for positively attributed prepared processes. Permanent, uncertain and
unsupported-platform refusals return immediately with cause. Duplicate validation,
launch exclusion, exact identity revalidation and legacy policies remain intact.
Registry-rescan cleanup is optional and deferred; no PID-kill fallback was added.

Historical prelanding verification on this macOS checkout (not rerun for this
documentation correction):

- Targeted normal and race regressions cover async durable-release failure,
  prepared absence/classification/inventory, authenticated timeout continuation,
  and native identity signal binding (race repetitions: three).
- `go test -race ./cmd/cercano -run '^TestDevelopmentLegacyProductionAdapter$' -count=3 -timeout=120s` passed after replacing its stale deadline assertion with immediate permanent refusal.
- `go test -race ./test/integration -count=1 -timeout=120s` passed.
- `env -u CERCANO_DISABLE_CREDENTIALS go test ./... -count=1 -timeout=180s`
  passed with default package parallelism; server production build and the
  disposable bridge test-binary build passed.
- A preceding full run exposed the stale legacy assertion. A second run with
  globally forced `CERCANO_DISABLE_CREDENTIALS=1` failed the ACP mock's missing
  GetCredentialPolicy attestation. Removing that override passed the isolated ACP
  test and the full gate. Neither result classifies the **earlier historical
  parallel failure**, which remains unclassified despite later full-gate passes.
- CLI focused recovery UI tests passed with race detection and a built server
  probe, including actual child exit and released/disconnected/live bindings.
  The latter fixture now expects its specific missing authenticated-resolver
  refusal instead of the superseded generic blocker text.

### Native acceptance boundary

[Full native recovery acceptance](full-native-recovery-acceptance.md) landed in
local main at `f736b1b43a214990ba0f4141a12edec1af8f8c11`, independently reviewed
with no blockers (reviewer: three normal and two race executions). It integrates
real CLI-model consent, authenticated native containment, guarded recovery, and
history hydration in one flow. It is **not** PTY/terminal, installed-launcher, or
real-inference acceptance. The earlier consent/adapter test substitutes process
seams and the CLI bridge starts from an already-exited source; those component
tests alone still do not establish full native acceptance.

The current `.github/workflows/ci.yml` job is `recovery-darwin-acceptance` on
`macos-14`. It retains native identity/prepared-inventory, authenticated-adapter,
and CLI proven-child-exit regressions and adds three normal and two race full
acceptance executions with instrumented children. Hosted execution remains
unobserved. Full server/CLI suites and the server build passed before landing;
these are historical results, not validation rerun for this docs correction.

macOS private API availability across supported OS/architecture versions remains
an open qualification gap; local Darwin runs and one configured runner do not
close it. Older hosts unable to freshly authenticate intentionally remain
fail-closed. No live user process/config/database was intentionally changed by
these tests; only disposable test-owned children and stores were used.
