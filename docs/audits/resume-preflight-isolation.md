# Resume serialization and bounded preflight

Resume journal serialization is keyed by conversation. A short registry mutex
protects token-channel gates and references covering holders and waiters.
Cancellation is checked before registration and after acquisition; zero-reference
entries are removed only if still the same registry entry. Deferred exact recovery
reconciliation runs before gate release. These gates do not confer ownership:
existing lifecycle, owner, and admission fencing remains authoritative.

Recovery confirmations use an independent mutex for atomic take/delete and
sweep/capacity/insert. The capacity remains 128, expiry five minutes, and consumption
remains single-use even on refusal. Published records are immutable; responses
clone stored offers. No callbacks or I/O run under the registry mutex.

Current observation supersedes the original List-based preflight: ordinary
resume uses bounded identity-only authentication and the trusted private record
for Store binding, never LifecycleList. Normal and recovery observation each
have an independent eight-slot pool and five-second budget; timed-out callbacks
retain capacity until joined. Late connections are closed. Identity is immutable
and does not wait on the lifecycle mutex. See
[bounded observation](../bounded-resume-observation-blocker.md).
Old hosts that cannot freshly authenticate still fail closed. No short deadline
is imposed over the complete stop/recover critical section.

## Limits

Callbacks must cooperate with context cancellation. Synchronous record/filesystem
lookups have context checks before and after, but cannot be forcibly interrupted.
No abandoned goroutine attempts to simulate cancellable filesystem I/O. A callback
that ignores context can delay its own conversation; it no longer holds a global
resume gate. Existing authority locks can still serialize critical ownership work.
No new authority policy, readiness changes, or live configuration/host operations
are included.

## Historical verification (serialization unit)

- The original global-lock reproduction was run before implementation; it exposed
  an unbounded List and an unrelated canceled resume parked behind the global lock.
- Converted isolation test checks bounded List, unrelated live progress, canceled
  resume, stable operation retry, and gate cleanup.
- Gate tests cover queued cancellation/exclusion, cleanup and token/cancel races.
- Pre-auth matrix covers resolve, dial, List, recovery resolve and selection,
  including translated Unavailable; child-deadline cases keep the caller alive.
- Consent tests cover concurrent duplicate consumption, capacity, expiration,
  requester/conversation refresh isolation, and cross-conversation refusal.
- `go test ./... -count=1` passed in source/server (macOS deprecated Security API
  warnings only). Production files were unchanged after this run.
- Final focused resume/live/offer/preflight tests passed five repetitions.
- Isolation and pre-auth matrix passed three race-enabled repetitions.
- Full internal/server and internal/broker suites passed, including a final
  race-enabled run after the test additions.

## Current status / supersession

Serialization and bounded identity-only observation landed in local main through
`f888165d`; the remaining audit diagnostics and retry fixes landed at
`a08f94f1f7afb66d866588ea86eea6e25c05905f`. Full native recovery acceptance
subsequently landed at `f736b1b43a214990ba0f4141a12edec1af8f8c11` following
independent review with no blockers. See
[bounded observation](../bounded-resume-observation-blocker.md) and
[full native recovery acceptance](../full-native-recovery-acceptance.md) for
current status and qualification limits. The verification above is historical;
its List test descriptions are not the current ordinary preflight contract.
