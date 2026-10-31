# Resume lifecycle track B (2026-10-31)

Branch `fix/resume-lifecycle-b`, isolated worktree `.worktrees/resume-lifecycle-b`,
base `87f4fe8d7dce096f3d29b393be295e51aa42d6bd`. Retained investigation
`.worktrees/resume-retired-host` / `b02eda30c7cb` was not changed.
No main merge, install, push, launcher progress changes, live endpoint/config/
history/credentials/models, or ambient signals. Tests use disposable stores,
synthetic external dependencies and test-owned loopback listeners/processes.

## Confirmed defects and changes

1. Opening a gRPC attachment stream was not acknowledgement of server enrollment.
   Immediate `ReleaseSession` could arrive before `BeginTerminalStream` registered
   the requester. Production ordinary-startup fixture reproduces this without
   release retries. The server now sends `cercano-terminal-attached: true` after
   enrollment; pinned SDK attachment waits for that header. Waiting is canceled
   by either caller context or client transport close. No attachment is fabricated
   by release, and no admission check is bypassed.
2. Missing requester now reports `requester terminal is not attached`; a genuinely
   competing terminal still reports `requester is not the sole attachment`.
   Dial/validation alone remains insufficient for terminal release.
3. An unrelated unavailable host listing previously prevented even ordinary
   acquisition of a cleanly released conversation. Discovery now distinguishes
   transport unavailability from invalid identity/authentication/records. When
   no retained owner is found, only transport uncertainty permits an attempt at
   the existing authoritative acquisition gate. Store locks and durable ownership
   records still reject owned/reserved conversations. Positive owners still use
   exact handover, with unchanged outstanding-work/attachment admission. Acquire
   responses must identify the exact destination/conversation and a nonempty epoch.

No edits to track A's `model.go`, `dev_host.go`, or CLI main setup; no shared diff
needed. Epoch/authentication/worker spawning and work retention policy unchanged.
The header is an additive server/client interface: pinned clients require the new
acknowledgement; an older host without it fails closed (or waits until cancellation
if it emits neither headers nor data). This does not promise mixed-version parity.

## Before / after executed evidence

Retained report established ordinary production explicit release vs transport-only
close, but used enrollment retries and did not establish dead-process behavior.
The new tests remove those retries from the immediate intentional-close case.

- With baseline server attachment and SDK files temporarily restored (new precise
  error retained), immediate production release failed repeatedly at `-count=30`
  with `requester terminal is not attached`. Fixed files restored afterward;
  all production resume tests pass at `-count=30`.
- With baseline `dev_host_reopen.go` temporarily restored, the opt-in real CLI
  reopen bridge failed on a test-owned unrelated refused endpoint despite a
  released row. Fixed implementation passes released and disconnected cases,
  repeated three times. No mocked lifecycle RPCs in this bridge.
- Ordinary production startup + authenticated client attachments: explicit release
  yields `released` at the original epoch; subsequent lifecycle acquisition gets
  a fresh epoch. A second real attached client blocks release without mutating the
  owned row; after that client's stream detaches the first can release.
- Transport-only disconnect and listener cleanup leave the owner `owned`; another
  host is refused while the original process still holds its capability lock.
- Separate child process deliberately exits without cleanup/release. Parent waits
  for process exit, opens the disposable store, observes `owned`, and acquisition
  fails with `crash recovery unsupported`, NOT file-lock contention. This proves
  the durable refusal independently of the original process's OS locks.

## Commands passed

From `source/server`:

```
go test ./cmd/cercano -run '^TestProductionResume' -count=30
go test ./internal/broker ./internal/server ./pkg/agentclient -count=1
go test -race ./internal/broker ./internal/server ./pkg/agentclient -run 'Test.*(Terminal|Drain|Attachment|Pinned|Session)' -count=1
go test -c -o /tmp/cercano-resume-lifecycle-b.test ./cmd/cercano
```

From `source/clients/cli`:

```
CERCANO_RESUME_PROBE_BINARY=/tmp/cercano-resume-lifecycle-b.test go test ./internal/ui -run 'Test(ReopenProduction|FindReopenOwner|DevHostOrdinaryReopen)' -count=3
go test ./internal/ui -run 'Test.*(Quit|Close|Release|Attachment|DevHost)' -count=1
```

`git diff --check` passed. Package regressions cover retained work/drain callbacks,
stale ownership, auth, terminal admission, and cancellation; race run covers the
affected interfaces. These are not a claim that all repository tests ran.

## Explicit limits / deferred policy

- The bridge runs ordinary production A but lifecycle-only B with a minimal reload
  callback. It proves real store ownership and real CLI discovery, NOT a second
  complete application startup, history/title hydration, or subsequent inference.
- Confirmed Ctrl-C UI tests and production immediate release tests are separate.
  There is no combined launched CLI/PTY A-create → B-start → confirmed Ctrl-C →
  drainage/retirement → `-r` history/title/subsequent-turn acceptance test here.
  Track A owns UI hydration. This track does not establish the cause of any
  historical user's missing attachment, nor release before attachment startup.
- Genuinely competing production attachments are tested. Outstanding work remains
  covered by broker/server regression tests, not a new production inference/task
  workload demonstration. No claim of universal containment or live migration.
- Dead still-owned recovery requires a new decision: who may fence/revoke the old
  durable epoch and reconcile unfinished work before creating a replacement epoch.
  Lock availability alone is not authorization. Existing production semantics
  explicitly forbid crash recovery; this portion is stopped, with no recovery
  policy or forced takeover added. Cleanly released recovery uses existing policy.
