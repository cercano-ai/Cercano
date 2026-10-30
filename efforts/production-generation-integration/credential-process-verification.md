# Isolated independent-process credential qualification — 2026-10-28

Scope: test-only qualification of the existing coordinator APIs from the 68dd98cb/f2b5627e slices. No production edits, adoption, activation, native backend, provider or network calls. This supplements (does not alter) credential-coordination-verification.md.

## Tests and evidence

`source/server/internal/hostsvc/credentials/process_qualification_test.go` launches independent copies of the test executable, selected by an exact helper test name. Every child explicitly receives `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`, checks those settings, and calls the coordinator directly without enabling policy.

- `TestCredentialProcessLogoutFencesDelayedPublication/login`: child snapshots the original revision and blocks on a pipe barrier; another process successfully logs out; delayed revision-bound active replacement returns ErrStale. Stale Admit refuses its callback and the exact logout record remains unchanged.
- The `refresh` subtest blocks the fake issuer after pending publication. A second refresh process signals actual lock contention (the filelock polling path consulting context Done after unsuccessful TryLock), then eventually returns ErrStale without calling its issuer. A separate nonblocking lock probe confirms refresh exclusion is held while commit exclusion is available. Logout completes before issuer release; delayed refresh cannot replace the logout marker. Pending and logout records contain no payload; logout bytes contain no secret, and remain identical after both refresh children finish.
- `TestCredentialProcessCommitExclusion`: while one process is inside Admit's synchronous callback, a separate process confirms commit lock contention and refresh lock availability. After callback release, logout succeeds.

Ordering uses pipe acknowledgments/releases and nonblocking OS-lock probes, not sleeps or elapsed-time assertions. The existing filelock implementation still polls internally. The contention observer deliberately depends on that implementation's Done call after a failed acquisition. Context deadlines (15s helper operations, 18s helper test, 20s process/barrier bounds) are failure bounds only. Cleanup closes stdin, cancels/kills owned children, and waits for reaping with a 5s bound. No child launches descendants.

## Fake backend contract and limits

TEST ONLY: one identity in a private temporary directory shared by the participating children. Replacement writes and closes a same-directory temporary file, then renames it over the record; fresh reads reopen the path. This assumes atomic whole-file replacement by rename on the tested local filesystem. Errors before rename preserve the old file. All participating mutations use coordinator locks; initial canonical active state is seeded only by the parent before children start, not by adoption.

No fsync, uncertain-write quarantine, injected replacement failures, crash/power-loss durability, native-store equivalence, remote revocation, production lock-root trust qualification, incompatible readers/writers, real provider dispatch, or platform-wide filesystem claim. Successful logout here means successful atomic fake replacement visible to subsequent processes, **not qualified durable native logout**. The Admit callback is a local barrier, not a transport. The login test exercises revision-bound coordinator publication, not Service/provider login wiring. Independent child processes and clean reopen visibility are covered; crash/recovery guarantees are not. APIs permitted these bounded tests without production changes.

## Executed verification

From `source/server`, all commands with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

- `go test -race ./internal/hostsvc/credentials -run '^TestCredentialProcess' -count=3` — PASS on Darwin (final version including contending-refresh child).
- `go test -race ./internal/hostsvc/credentials -count=1` — PASS, including existing tests. Some existing narrowly scoped service tests enable policy with memory fakes; the new process tests and all their children do not.

No full server/CLI suite or native qualification executed. Existing adoption/activation blockers remain unchanged.
