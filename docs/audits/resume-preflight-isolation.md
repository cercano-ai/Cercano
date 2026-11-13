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

Source resolution (including recovery revalidation), authenticated dial, and List
use five-second child contexts; shorter caller deadlines win. Phase expiration
is checked before classifying adapter errors, including translated Unavailable.
Exact authenticated List identity remains the boundary before the existing shared
three-second cooperative force/legacy budget. No short deadline is imposed over
the entire stop/recover critical section.

## Limits

Callbacks must cooperate with context cancellation. Synchronous record/filesystem
lookups have context checks before and after, but cannot be forcibly interrupted.
No abandoned goroutine attempts to simulate cancellable filesystem I/O. A callback
that ignores context can delay its own conversation; it no longer holds a global
resume gate. Existing authority locks can still serialize critical ownership work.
No new authority policy, readiness changes, or live configuration/host operations
are included.

## Verification

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

This unit is checkpointed on its isolated branch only; not landed or pushed.
