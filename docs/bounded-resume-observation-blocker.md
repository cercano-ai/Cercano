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

## Disposable verification

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

## Remaining audit review — 2026-11-13

Baseline bounded identity-only observation is landed in local main `f888165d`.
The remaining audit branch `fix/remaining-recovery-audit` is not landed or pushed.
It adds correlated, safe asynchronous terminal release failure diagnostics without
clearing unresolved blockers/ownership, and retries final prepared-writer absence
only for positively attributed prepared processes. Permanent, uncertain and
unsupported-platform refusals return immediately with cause. Duplicate validation,
launch exclusion, exact identity revalidation and legacy policies remain intact.
Registry-rescan cleanup is deferred; no PID-kill fallback was added.

Verification on this macOS checkout:

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

The added `recovery-darwin-components` macos-14 CI job runs native identity signal
and prepared-inventory components, authenticated-adapter regressions, and the CLI
proven-child-exit bridge. CI execution itself has not been observed here. This is
**not** production CLI consent -> authenticated native stop -> recovered UI/state
end-to-end acceptance. The consent/adapter test substitutes process inspection,
stop and absence seams; the CLI bridge starts from an already-exited source.
Neither may be presented as full native acceptance.

A complete acceptance harness still needs an explicit choice: a disposable,
canonical prepared-generation launcher running real production CLI/hosts and
native stop, versus extending the test-host bridge with launcher/authentication
wiring (still not terminal acceptance). No production test mode or alternate
identity authority was invented to bridge that gap. macOS private API availability
across supported OS/architecture versions remains a qualification gap; one local
Darwin run and a single proposed runner do not close it. Older hosts unable to
freshly authenticate remain fail-closed. No live user process/config/database was
intentionally changed; only disposable test-owned children and stores were used.
