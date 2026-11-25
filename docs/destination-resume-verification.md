# Destination-driven resume verification

Historical branch: `fix/destination-resume`, based on then-main `1c070493acb2`.
Destination-driven recovery and bounded identity-only observation are included
in local main through `f888165d`; the historical commands below are not fresh
verification of this review branch. See the current audit addendum in
[bounded observation](bounded-resume-observation-blocker.md).

## Reproduction and correction

Before production edits, `TestResumeNeverCallsLiveLegacySource` failed because
ordinary reopen contacted historical A for `LifecycleList` (one RPC), receiving
`Unimplemented`, instead of returning B's recovery blocker. This is the smallest
reproduction of the historical-owner routing defect; it does not claim to
reproduce the user's exact old-binary `LifecycleForceRelinquish` response.

Reopen now acquires only on the selected destination. On refusal it asks that
destination's `LifecycleRecover` to inspect durable ownership. Only verified
absence permits a confirmation offer; confirmation repeats recovery with the
exact inspected owner/epoch. No source `ForceTransfer` or historical host scan
remains in this flow. The default selection for new conversations is unchanged.
Ready local ownership can be reacquired after response/client loss without
creating another epoch or requiring existing attachments to disappear.

`LifecycleRecover` retains the existing production `RecoveryGuard` and
`RecoverDeadOwner` path: launch exclusion, exact-source absence verification,
conversation-tree locks and transactional rechecks, interrupted unfinished tasks
without replay, preserved results, fresh epoch, and idempotent recovery receipt.
No schema, journal, or lock/epoch bypass was introduced.

## Final bounded verification

Using `/opt/homebrew/bin/go`, a newly isolated HOME/XDG under
`/tmp/destination-final-rno7hej3`, TMPDIR=/tmp, a minimal allowlisted environment,
CERCANO_DISABLE_CREDENTIALS=1, GOTELEMETRY=off, GOTOOLCHAIN=local,
GOPROXY=off, GOSUMDB=off, explicit cached modules at
`/Users/bryancostanich/go/pkg/mod` and GOCACHE at
`/tmp/cercano-destination-go-cache`:

- Server: `go test ./internal/server ./internal/conversation -count=1 -timeout=90s`
  passed (4.743s / 11.796s).
- Server: `go test ./cmd/cercano -run '^TestDevelopmentRecoveryRegistryExclusion$' -count=1 -timeout=30s`
  passed (0.754s).
- Built disposable server test binary with `go test -c -o <isolated-root>/server.test ./cmd/cercano`.
- CLI with CERCANO_RESUME_PROBE_BINARY pointing to that test binary:
  `go test ./internal/ui -run 'Test(DestinationResumeAfterActualProcessExit|ReopenProductionReleasedAndDisconnected|ResumeNever|DeadOwner|ForcedResume|LauncherResume|DevHostOrdinary|ResumeKeeps)' -count=1 -timeout=90s -v`
  passed (1.677s), including released/disconnected/live subprocess cases.

The actual-exit bridge has A append history and exit without releasing its owned
row. B uses real authenticated lifecycle RPC, the production registry/launch
exclusion guard with a verifier tied to Wait's actual child exit, and the real
SQLite recovery path. CLI confirms, binds and reconnects on B. The bridge then
persists a synthetic next turn on the same conversation and asserts both turns
and a fresh epoch. The historical endpoint is unavailable. This is not a model
inference, PTY, or full UI-history-rendering test. Existing store/server tests
cover stale writers, interrupted tasks/results, atomic failures, idempotent
confirmed retry, and ready-owner reacquisition. Confirmation tests cover default
No (Enter), n, Escape, yes and stale callbacks.

## Remaining boundary

A live or unknown owner is NOT safely recoverable by this path. The destination
reports that source death was not verified and safe live-owner fencing is
unavailable; it never calls a legacy source for relinquishment. Unreachable RPC
is not death evidence. Missing or incompatible local registry/launch evidence
also blocks recovery. No live processes are signaled, no locks are deleted, and
no external-effect cancellation guarantee is made. The process-exit bridge
injects only the death verifier to avoid inspecting ambient processes; it does
not qualify the Darwin kernel process enumerator end-to-end. Full repository
suites and real model execution were not run. This checkpoint is for independent
review, not landing.

## Combined integration final gates — 2026-11-25 (awaiting rereview)

Validated production tree: `e4f132d3e009b627e11ed4610947436cb961785f`,
`fix/reviewed-integration-20261125`. This follow-up changes only the two
production-scheduling tests, the ACP protocol fixture, and this document.
**Not landed or pushed; independent final test-diff review is still required.**
The older “full repository suites ... were not run” statement above describes
that historical checkpoint. Full server and CLI module normal suites now pass;
this does not qualify every repository platform or full native application use.

### Reproduced failures and bounded correction

Original integration tests reproduced both old cancellation expectations:
`production_prepared_scheduling_test.go:165` (“cancelled work lost closure
protection: <nil>”) and `production_scheduling_test.go:280` (“cancelled write
lost blocker: <nil>”). An exact original-file Go overlay reproduced these again
without reverting the working files. Compaction now distinguishes a proven
inference-only error from persistence failure: `Advance` must return the
summarizer error before `SaveCompaction`, and the write-outcome observer must
remain clean. Partial progress, uncertain commits and reported write failures
are not made releasable by these test changes. No production safety defect was
found in the exercised paths; no production code changed.

The common-factory fixture now counts attempted recap/compaction writes (zero
on inference cancellation), checks clean `WriteFailure`, joins both generators,
and asserts no remaining local/host tickets or unresolved work before closure.
The real prepared-server constructor fixture holds the provider until explicitly
unblocked, verifies join cannot finish early, checks cancellation and clean write
outcomes, then releases and reacquires from the other prepared host with a new
epoch. This is durable ownership reacquisition, not a new model turn or UI resume.
The constructor fixture uses the exact validated store and production scheduling
binding, unlike the common-factory fixture's manually supplied admission bridge.
Neither focused fixture itself runs the complete `RunPrepared` entrypoint.
Full module tests additionally include `TestInstalledProductionColdHost`; these
are distinct layers, not one combined native cancellation/resume journey.

Existing `TestCompactionInferenceAndSQLiteOutcomes` remains unchanged and passed
20 race repetitions, including actual SQLite abort, ownership refusal, uncertain
commit and cancellation with a reported write failure. Those cases preserve
fencing; no blanket cancellation exemption or negative-test removal was made.

ACP reproduced on the clean detached disposable baseline worktree at
`cbd16185c270e6c46f9d5f4669c8aa2a38521fc9`: both allow/cancel failed at
`agentclient_test.go:87` because `GetCredentialPolicy` was unimplemented under
`CERCANO_DISABLE_CREDENTIALS=1`. This is preexisting, not caused by the combined
fixes. The in-memory protocol mock has no credential store or secret access;
it now honestly returns `CredentialsDisabled: true`, and its test explicitly
requires isolation. Production validation and existing policy-negative tests
are unchanged (included in the passing full server suite).

### Commands and results

Artifacts are local under `/tmp/cercano-final-gates.DwvdA8` (`R` below).
HOME and XDG config/data/cache/state and TMPDIR point beneath R; credentials
are disabled and GOTELEMETRY=off. GOPATH/GOMODCACHE/GOCACHE retain the existing
Go tool caches, resolved before HOME changes. This is runtime directory
isolation, not a claim of a fully allowlisted outer environment. The installed
cold-host subprocess fixture applies its own environment allowlist. All final
commands below used an outer 900-second limit.

From `source/server`, with
`F='TestProduction(PreparedSchedulingOwnership|SchedulingCommonFactory)|TestGRPCAdapterPermissionAndCancel'`:

```sh
go test -overlay "$R/original-overlay.json" ./cmd/cercano ./internal/acp -run "$F" -count=1 -timeout=60s
# Expected exit 1: exact original three test files from integration HEAD.
go test ./cmd/cercano ./internal/acp -run "$F" -count=20 -timeout=300s
go test -race ./cmd/cercano ./internal/acp -run "$F" -count=20 -timeout=300s
go test -race ./internal/hostsvc/persistence -run '^TestCompactionInferenceAndSQLiteOutcomes$' -count=20 -timeout=300s
go test ./... -count=1 -timeout=300s
go build -o "$R/cercano-server" ./cmd/cercano
```

All non-overlay commands exit 0. Normal focused: 10.580s / 0.391s;
race focused: 90.700s / 1.463s; persistence negative/positive race matrix:
54.910s. Full server outer elapsed: 1m43.899s, exit 0. Build exit 0.
Earlier full server attempt failed only when the installed fixture's offline
build lacked cached modules (`GOPROXY=off`); final run used the populated cache
without changing that fixture or weakening credential isolation.

From the detached baseline's `source/server`, same credentials-disabled runtime
isolation: `go test ./internal/acp -run '^TestGRPCAdapterPermissionAndCancel$'
-count=1 -timeout=60s` exits 1 with both missing-policy failures (`acp-base.log`).

From `source/clients/cli`:

```sh
go test ./... -count=1 -timeout=300s
go build -o "$R/cercano-cli" .
```

Both exit 0; full CLI outer elapsed 9.739s. Logs: `red-reproduced.log`,
`repeat.log`, `race.log`, `negative-race.log`, `server-full-cached.log`,
`cli-full.log`, `server-build.log`, `cli-build.log`. Builds are temporary.
No full repository race suite, real model/secret access, PTY/full native UI,
Darwin process-enumerator acceptance, or external-effect cancellation guarantee
is claimed. Historical recovery and process-fixture limitations above remain.
Source worktrees and the dirty root were not modified; no live host state was
used. Review should specifically check inference-vs-write classification,
join/ticket assertions, durable new-epoch acquisition, and fixture-only policy
attestation before any landing decision.
