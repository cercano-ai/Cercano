# Compaction scheduling producer slice — blocked before implementation

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
