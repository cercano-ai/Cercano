# Compaction scheduling producer slice — opt-in implementation verified

The historical blocked probe below is superseded by the implementation evidence at the end.

Baseline inspected: `56399c38794696bbe980f8cc8e4e8cbf68968257`.

Read the approved plan/specification and bounded compaction, broker, ownership,
and persistence callback seams. Actual scheduler path is
`source/server/internal/compactiongen`, not repository-root `internal/compactiongen`.

## Reproduction artifact

Added `TestLegacyScheduleContextLossAndTimerGap` in
`source/server/internal/compactiongen/owned_schedule_test.go`.
It queues a one-hour legacy timer, observes that the worker wait has no registered
work, explicitly fires the timer, and holds the fake elision callback at a channel
barrier. The callback checks that the legacy API carries neither parent metadata
nor parent cancellation. No inference, credentials, persistent stores or child
processes are used. This is a compatibility probe, not proof of an owned API.

Attempt from `source/server`:

```
CERCANO_DISABLE_CREDENTIALS=1 go test ./internal/compactiongen -run TestLegacyScheduleContextLossAndTimerGap -count=1
```

Result: tool execution failed before compilation:
`fork/exec /opt/homebrew/bin/go: no such file or directory`.
The probe is **unexecuted**; no reproduction pass, focused-suite pass, formatting
verification or race result is claimed. Implementation stopped to preserve the
required reproduce-before-fix gate. No toolchain installation or host changes.

## Inspected integration constraints

- Legacy `Schedule` uses an ID-only timer and `context.Background`; busy passes
  and remaining-backlog passes reschedule through that API.
- `startWork` registers only when execution starts; `Close` stops legacy timers
  and cancels/waits active work. A new owned seam must count admission before
  queueing, not merely wrap `CompactNow` after timer expiry.
- Broker `WorkTicket.Advance` supports continuous authority;
  `PersistenceFailed` requires `PersistenceWork` and keeps `Complete` busy.
  `ResolvePersistence` is explicit durable reconciliation, not automatic retry.
- `conversation.WithOwnership` carries actual store-bound capabilities. Merely
  passing broker owner strings cannot substitute for that capability context.
- Persistence `SetCompactionGenerator` respects `sharedWritesFrozen` and wires
  the elision and usage callbacks. Usage writes report through the existing
  `WithWriteFailure` / `ReportWriteFailure` outcome seam despite their void API.
  An owned integration must observe that seam before completing a ticket.

No lifecycle API conflict was established. No production implementation,
prepared-generator activation, ownership/security change, main/watcher/shutdown
edit, migration, native credential access or provider call occurred. The Phase 2
producer row is blocked for this bounded slice; it does not claim completion of
recap, metering, rollover or other producers.


## Resumed implementation — 2026-10-27

Baseline HEAD at verification: `a0c150db71eefbecbb5221122fc32975d3297031`.
Scoped implementation is complete; the broader Phase 2 producer row remains
**in_progress**, not complete. Prepared generators remain globally disabled;
no production composition, main, shutdown, watcher or provider lifecycle changes
are part of this slice. Other concurrent worktree changes are excluded from the
explicit-path checkpoint.

- `compactiongen.ScheduleOwned` admits exact authority before queueing and keeps
  one broker ticket through timer waits, busy/backlog retries, inference, store
  writes and synchronous callbacks. It preserves the admitted capability context.
  Independent calls intentionally do not debounce across caller authority.
- Existing ID-only `Schedule` remains compatible. Internal rescheduling detects
  owned passes instead of falling back to the background-context legacy timer.
- `persistence.ScheduleOwnedCompaction` is the opt-in concrete bridge: it wraps
  the existing void callback failure observer and feeds its outcome into the
  producer. Failed usage writes suppress owned pass success logs and completion.
  It does not install or enable a prepared generator.
- Cancellation/drain waits for accepted workers and callbacks. Queued cancellation
  releases tickets without writes. Errors/panics after execution starts retain
  conservative persistence blockers; cancellation/drain never reconciles them.
  `ScheduledWork` exposes its ticket for explicit durable reconciliation, not
  automatic replay. A producer drain is not broker readiness.

### Tests actually run

From `source/server`, with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

```
go test -race ./internal/compactiongen ./internal/hostsvc/persistence -count=5
```

PASS (both packages, final run exit 0). `gofmt` ran on changed Go files.
This run also executes the legacy reproduce-before-fix compatibility probe.
No Go-launch failure occurred in the resumed execution.

Added deterministic channel-barrier tests cover a busy retry retaining the same
admission, fake inference with caller metadata, cancellation while a callback
remains active, drain refusal while that callback runs, and retained ambiguous
outcomes after drain. One-hour queued timers establish counted waiting without
sleep-based assertions; zero-delay timers and barriers drive execution.

The persistence integration test creates a temporary coexistence SQLite store,
acquires a real store-bound capability, uses broker exact-owner admission,
executes the producer through the persistence bridge, and verifies durable
compaction plus context-usage rows. It rejects stale epochs, verifies broker
busy during execution, performs another SQLite write while the producer is held
at a callback barrier, and injects a real rejected unowned usage write through
the void callback. That failure remains a broker readiness blocker after the
producer completes. Fake inference tests use an in-memory store; the SQLite
integration uses tool-elision-only fake callbacks. Neither calls a provider.

Qualification limits: these are bounded producer/bridge integration and race
tests, not full production-composition, two-host compiled A/B, native inference,
credential, migration or activation acceptance. Recap/metering/rollover and the
remaining acceptance gates are not completed by this compaction slice. No SQL
transaction is introduced around inference or callbacks. No live/user data,
launcher, native credentials, provider transports, push or merge were used.
