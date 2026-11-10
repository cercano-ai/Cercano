# Old-host-compatible recovery implementation

Execute the approved spec in spec.md. Whole-old-host interruption with disclosed collateral impact is approved design, not an unresolved choice. No live recovery, production process mutation, push, or unrelated Windows changes. Implement in an isolated worktree after approval. All results must distinguish fixtures, actual old-binary interoperability, and live deployment.

## Phase 1 — Reproduce the old-owner failure and map cleanup

Objective: establish the smallest failing end-to-end recovery journey before modifying production behavior. Inspect the current tree rather than relying on prior task reports. Files: server resume/recovery tests, command process-control tests, client/UI recovery tests, and protocol consumers. Tests: actual pinned old implementation lacking new APIs, isolated state and child processes, multiple destination journals, and a deterministic loopback provider. Do not rebuild or execute a live user's dirty generation.

- [x] Trace protocol consumers and identify redundant orchestration, retained safety fixes, and compatibility-required old controls.
- [x] Establish a reproducible old-binary provenance and offline build fixture; state differences from the incident binary.
- [x] Reproduce unsupported-probe and multiple-journal failures with the old fixture and current destination.
- [~] Trace old host/worker process ownership, shutdown behavior, launch participation, and store compatibility; probe containment and relaunch exclusion in disposable state.
- [~] Add the failing old-owner-to-new-owner persisted-turn acceptance journey and precise no-mutation assertions for declined recovery.

## Phase 2 — Exact source interruption and writer containment

Objective: implement the approved fallback without trusting stale PIDs or requiring new source RPCs. Files: command-level source resolution and process controls, server recovery boundary, agentclient APIs as needed. Tests: source identity changes, surviving workers, concurrent launch, cancellation, other hosts, and sibling impact.

- [ ] Bind fresh confirmation to source identity, conversation epoch, selected destination, and host-wide interruption disclosure.
- [ ] Implement verified launch exclusion and exact source/worker attribution using existing supported OS primitives where adequate.
- [ ] Implement bounded stopping and death verification for attributed processes, with explicit refusal on uncertain attribution or containment.
- [ ] Verify unrelated hosts remain untouched and source relaunch cannot race recovery.
- [ ] Cover timeout, cancellation, PID reuse, stale approval, and surviving writer failures without ownership reassignment.

## Phase 3 — Transactional ownership recovery and retry

Objective: recover owned or reserved state only after verified containment, without stealing an already-claimed epoch or replaying interrupted effects. Files: conversation ownership/dead-owner code, server recovery coordinator, persistence and task reconciliation tests.

- [ ] Extend recovery admission to exact owned and reserved cases under stable locks and transactional owner/epoch rechecks.
- [ ] Serialize reserved-state recovery against late destination claims; reject recovery after another destination acquires ownership.
- [ ] Preserve committed history/results, mark interrupted work accurately, and retain explicit uncertain-outcome handling.
- [ ] Preserve retry operation identity across timeout and response loss; return ready only for current ready ownership.
- [ ] Test stale epochs, independent descendants, uncertain commit, response loss, duplicate retries, and late claims.
- [ ] Test and document completion-notification behavior separately from prohibited interrupted-task/action replay.

## Phase 4 — Single reopen flow and protocol cleanup

Objective: connect normal UI recovery to the replacement and remove redundant repair paths without losing safety invariants. Files: server resume/discovery/retry/takeover orchestration, agentclient wrappers, CLI reopen/recovery UI, proto definitions/generated consumers, mocks and docs.

- [ ] Route recovery from current durable ownership; use supported old controls only for safe cooperation, never as discovery probes.
- [ ] Make unsupported optional RPCs reach the explicit compatible fallback rather than abort reopen.
- [ ] Remove journal-count ambiguity as a prerequisite to independently authorized death-proven recovery; keep journals as evidence only.
- [ ] Consolidate user confirmation and result handling into one recovery journey, including actionable failure stages and host-wide impact.
- [ ] Remove superseded protocol families and consumers identified in Phase 1; retain required compatibility controls and reusable safety primitives.
- [ ] Reserve removed protobuf field/RPC-message identifiers as appropriate, never reuse field numbers or names; regenerate using repository tooling.
- [ ] Port useful regression tests and correct misleading documentation while retaining historical evidence as historical.

## Phase 5 — Verification, independent review, and checkpoint

Objective: establish the approved old-host recovery outcome, not merely current-to-current protocol success. Files: affected server/client/command integration tests and effort verification notes.

- [ ] Pass the actual old-process, multiple-failed-journals, new-host recovery journey and exact successful persisted turn assertions.
- [ ] Run focused normal/race suites for ownership, admission, source control, recovery, agentclient and CLI; confirm nonzero selected tests.
- [ ] Run interface compilation and integration checks across affected modules and supported platforms; record untested platform boundaries.
- [ ] Independently review process attribution, writer containment, late claims, consent, uncertain writes, protocol removal and compatibility evidence.
- [ ] Repair concrete findings with failing probes and rerun affected tests.
- [ ] Checkpoint solved units with explicit paths; preserve unrelated files and worktrees. Do not push or merge without authorization.
- [ ] Report exact commits, executed checks, retained limitations and deployment prerequisites; do not claim the user's live conversation recovered.
