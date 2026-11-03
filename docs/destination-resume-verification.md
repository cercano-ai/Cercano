# Destination-driven resume verification

Branch: `fix/destination-resume`, based on local main `1c070493acb2`.

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
