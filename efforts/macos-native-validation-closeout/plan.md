# macOS session-safe upgrade closeout

Execute the scope approved in spec.md. Deliver minimal integration, focused regression tests, one native rehearsal, and a human acceptance checklist. Reuse existing evidence. Do not duplicate Homebrew transactions or release qualification. Engineering tasks below are complete within the bounded scope recorded in [closeout verification](verification.md#engineering-closeout-and-human-handoff--2026-11-05); all human checks and separate release gates remain pending. Historical successes remain baseline evidence, not substitute execution results. No publication, push, landing, host installation changes, or automatic old-keg retirement is authorized.

## Phase 1 — Establish the execution baseline

Objective: preserve active work and select the smallest remaining gaps. Files: this effort's planning documents, existing wt-native-packages documentation and tests; create verification.md during execution. Tests: inspect existing results and test selectors rather than rerunning the whole historical matrix.

- [x] Delegate repository inspection to confirm the active worktree, branch, checkpoint, unrelated changes, and presence of the installed-worker fix. Reconcile these planning documents with that worktree through a scoped checkpoint without merging unrelated main changes or discarding existing work.
- [x] Verify the disposable guest is accessible and identify owned synthetic state and effective Homebrew configuration. Read-only inspection first; no host installation changes.
- [x] Record which existing tests and native evidence are reusable, with precise remaining assertions. Confirm whether a historical monolithic package actually reached users requiring migration; absence of evidence is not proof it did not.
- [x] Record the minimum resources needed for deterministic turn tests and local-runtime investigation. Obtain any missing authorization before downloads, model runs, or credentials are used.

## Phase 2 — Complete ordinary dispatch and session binding

Objective: close the application path, not add an updater. Candidate files: source/server/internal/worker/installed_keg_test.go, source/server/internal/server/installed_worker_test.go, source/server/cmd/cercano/production_composition.go, ordinary_generation_host.go, source/server/pkg/agentclient/client.go, reconnect.go, installed_reuse_test.go, and existing generation admission/session code only where needed. Tests: successful fresh-worker dispatch, stable A reconnect, eligible new B session, and mixed-version compatibility.

- [x] Build the smallest deterministic successful-turn integration case through ordinary server dispatch using isolated state and a controlled test service. Observe current behavior and record a failing probe before changing code.
- [x] Trace reusable admission/routing mechanisms and identify the minimum safe bridge for ordinary installed sessions. Do not assume prepared-generation authority follows from keg selection. If there is a consequential architecture choice, present that specific decision for approval before implementing it.
- [x] Implement only the missing bridge, preserving authentication and store authority; prove a later fresh worker uses A after public selection changes to B.
- [x] Add focused coverage showing existing A sessions reconnect to A while eligible new sessions reach B, without stopping A. Reuse missing/replaced executable rejection tests rather than duplicating them.
- [x] Cover compatible and incompatible CLI/agent version handling and installed-but-deferred status using existing mechanisms. Add only necessary installed/session identity and blocked-reason diagnostics.
- [x] Test shared-state access or interrupted Cercano admission only where the changes introduce a concrete risk. No generic recovery framework or storage redesign.
- [x] Run affected package tests and integration tests; use race detection for changed concurrency paths. Delegate an independent bounded review, resolve concrete defects with reproductions, and checkpoint explicit paths without pushing.

## Phase 3 — Establish the local-runtime support boundary

Objective: answer the real dependency-safety question without creating a custom installer. Candidate files: docs/homebrew-worker-selection.md, source/server/internal/localruntime/llamaserver/{provider.go,detect.go}, source/server/internal/modelhost/, and source/server/cmd/cercano/ordinary_upgrade_test.go. Tests: only those necessary for the actual supported runtime and later-use behavior.

- [x] Trace the selected llama executable and its actual library/backend/resource/model dependencies; identify which existing native-manager policies and Cercano bindings protect them across external upgrade/cleanup and which do not.
- [x] Reuse compatibility and nonreplacement tests. Probe a concrete uncovered case before applying any fix; do not infer complete dependency retention from one binary or a running process.
- [x] Implement the minimum supported protection if existing approved mechanisms suffice. If a consequential mechanism choice remains, or protection cannot be guaranteed, stop for approval of the specific design or explicit support limitation rather than expanding scope or claiming safety.
- [x] Record the supported boundary and, where supported, exercise idle-session later runtime use plus deferred incompatible activation using the smallest suitable test. Keep broader inference usability in the human checklist.
- [x] Review changed behavior and documentation, run focused tests, and checkpoint the slice. Mark unaccepted safety limitations blocked rather than complete.

## Phase 4 — Run one native upgrade rehearsal

Objective: connect the current implementation to Homebrew retention evidence. Files: a bounded reproducible rehearsal script under this effort if needed, verification.md, and only affected packaging fixtures. Tests: current-code versioned A/B packages in the disposable guest, not Homebrew fault injection.

- [x] Build current-code A/B fixtures containing the fixes; record source revision and artifact hashes. Do not manufacture a source change for a compatibility claim.
- [x] Establish isolated synthetic state, effective agent cleanup exclusion, and an ordinary A agent/conversation. Record how a genuinely fresh worker will be distinguished from warm-worker reuse.
- [x] Upgrade externally using Homebrew and exercise automatic and explicit cleanup. Assert A remains present by identity; account for the known cleanup refusal message with exit 0 rather than relying on exit status alone.
- [x] Have A complete a normal dispatched turn through a fresh A worker; verify worker identity. Verify a new eligible session reaches B and the existing A conversation still works.
- [x] Rerun package ownership/lifecycle checks only if relevant packaging changes invalidated existing evidence. Add the actual historical migration only if its supported-user requirement was established in Phase 1; otherwise document omission or the unresolved evidence question.
- [x] Record failures honestly, preserve logs/hashes, terminate only owned processes, and document remaining guest packages/configuration. Review evidence against assertions and checkpoint it without claiming full human or release acceptance.

## Phase 5 — Hand off human acceptance

Objective: make remaining human testing straightforward and separate from engineering completion. Files: this effort's human-acceptance.md and verification.md; link existing release qualification rather than duplicate it. Tests: human-executed workflows, explicitly pending until results are supplied.

- [x] Write a short checklist for normal and in-flight conversations, subsequent turns/delegation, reconnects, simultaneous A/B sessions, local inference after idle within the supported boundary, and understandable version/deferred-state reporting.
- [x] Include expected outcomes and a compact failure-report template with versions, session/worker identities, reproduction steps, and relevant logs, excluding secrets.
- [x] Hand off the next real changed-source release transition and final signed-package smoke test to human testing and the existing release process. Do not require new credentials/runners or another release pipeline for this engineering closeout.
- [x] Summarize engineering results, approved limitations, pending human checks, and separate release gates. Inspect final delegated review and repository results before reporting completion; keep human checks pending until actually performed.
  Engineering completion stops at the handoff above. Any human-reported defect is handled with a concrete reproduction and scoped fix, not by expanding this effort into an exhaustive platform qualification program.
