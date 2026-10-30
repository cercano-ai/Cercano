# Forward-only launcher flow — offline evidence (2026-10-29)

## Expected lifecycle (stated before rehearsal)

A remains alive with its retained sessions and outstanding work while B starts.
Only authenticated-ready B becomes the new-session target. Existing A reconnects
and newly born A workers stay pinned to A and do not publish the target. Failed B
readiness preserves A's target. Disconnect does not close a session. After A's
sessions are explicitly released/transferred and its work drains, existing
retirement accounting shuts A down automatically; B remains usable.

## Implementation boundary

The normal new-session launcher serializes generation starts/target publication
and atomically publishes private `current-target.json` only after authenticated
readiness, session acquisition, and selection persistence. The record has no
conversation/epoch authority. Existing exact-host reconnect/control/worker paths
are unchanged and do not write it. Normal launches still use the existing source
publisher and generation selection; this is not a second source-version resolver.

Both ordinary and fixture host owners poll this record and call the existing
`RetireCoexistence`; admission, sessions, work, cleanup and shutdown remain owned
by the existing lifecycle. Missing/invalid/mismatched records cannot authorize
retirement. Invocation-local startup predecessor identity prevents an unselected
B from retiring itself while A is still current; capture precedes host readiness
publication. There is no durable supersession ledger, reversal, rollback, or
source-reversion policy.

## Verification

All commands used `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`.

- Focused regression: the carried-in pre-fix test reported missing target
  publication. Independently rechecked the updated test against baseline
  `08a9b615b6cf3eee2f6587ecc37459ed97b46fcc` launcher using a disposable Go overlay:
  expected failure, `invalid current target` still pointing to A after successful
  B launch. Local output: `.forward-flow-evidence/baseline-regression.txt`.
- PASS: `go test -race ./cmd/cercano -run
  'TestCurrentTarget|TestDevelopmentLaunchAuthenticatedConcurrentReuse|TestDevelopmentHostRetirement'
  -count=1 -timeout=90s` (server module). Tests include unready B preserving A,
  blocked readiness preserving A, concurrent ready launch publication, unreachable
  launch preserving the record, idle startup candidate survival, current idle
  host survival, disconnected retained sessions/work blocking retirement, later
  A work admission, and automatic drained retirement.
- PASS: `go test ./cmd/cercano ./internal/server ./internal/broker
  ./internal/conversation ./pkg/agentclient -count=1 -timeout=180s`.
- PASS on final source: `python3 efforts/dev-host-generations/everyday-launcher-compiled.py
  --repo . --evidence .forward-flow-evidence-final`.
  This is the actual shell/publisher/compiled ordinary host/compiled CLI PTY path,
  with tagged synthetic model/MCP fixtures, isolated configuration/database/runtime.
  It adds a normal fresh B session, verifies A control and later A worker do not
  change the target, and removes explicit A retirement. It still verifies busy
  reopen refusal, retained task completion, explicit handover/resume with history,
  MCP, UpdateConfig, watchdog, and fresh B durable work after automatic A exit.
  Report has 29 steps, `passed=true`, cleanup `remaining=[]`.
  Report SHA-256: `8f58336bffc74030bb9fcb8c8715f62b6549ecf281518ec6bb48264199bb119b`.

## Scope and retention

Bounded offline acceptance only: not live production cutover, provider integration,
or source-reversion semantics. The optional worker-local review-delegation mode
was not run; the ordinary later-worker A identity/build/hash assertions were run.
No install, live credentials/history/runtime mutation, unrelated process stop,
push, or landing. Cleanup targeted only recorded disposable fixture processes.

Retain the integration worktree for review and its pre-existing unrelated plan,
attachment/routing artifacts, plus local raw forward-flow evidence. The checkpoint
includes only explicit implementation/test/rehearsal/documentation paths; unrelated
changes and raw local evidence are not staged.
