# macOS session-safe upgrade closeout

## Status and purpose

Draft scope, revised after user review to avoid duplicating Homebrew or existing release qualification. This effort delivers the smallest necessary implementation, focused regression tests, one native A-to-B rehearsal, and a short human acceptance checklist. It closes the macOS engineering slice, not the entire cross-platform effort or all release qualification.

Implementation currently lives in wt-native-packages on feat/native-packages-session-safe. Preserve and reconcile that work before execution. This specification is not permission to publish, push, land, change the developer's live installation, or use production credentials.

## Baseline

Existing disposable-guest evidence covers split CLI/agent installation, the convenience package, command ownership, a same-source version-only package lifecycle, and old-agent-keg retention under an effective persistent Homebrew cleanup exclusion. Fresh A and B workers were launched directly. Those native fixtures predate the worker-selection fix and do not prove ordinary agent dispatch or conversation continuity.

Current installed-worker selection pins the canonical agent keg and fails closed if it disappears or changes. Focused tests cover that selection, but stop short of a successful worker turn. Ordinary clients still reuse an existing agent; changing the public executable to B does not itself route new sessions to B. Separately installed llama and its dependencies are outside the agent-keg exclusion.

Reuse evidence and code in the active worktree: docs/homebrew-worker-selection.md, efforts/cross-platform-updates/verification.md, source/server/internal/worker/installed_keg_test.go, source/server/internal/server/installed_worker_test.go, and source/server/pkg/agentclient/installed_reuse_test.go. Historical cross-platform plans are context, not authorization to expand this scope.

## Deliverables

### 1. Minimal session-safe implementation

Connect existing installed-worker selection to successful ordinary agent dispatch. Add only the admission, routing, and reconnect integration needed to keep existing A sessions and later workers on A while eligible new sessions reach B. Preserve existing authentication, ownership, and store-admission boundaries; executable pinning is not prepared-store admission.

Reuse existing shared-state compatibility safeguards. Add store/configuration/credential tests only where new routing or coexistence code changes access, or where a concrete safety gap is found. Refuse incompatible activation before mutation rather than forcing old sessions to stop.

Provide enough diagnostics to identify the installed version, the session-serving version, and why activation is blocked. Do not build a general update-status subsystem.

### 2. Focused automated regression coverage

Protect the critical invariants:

- An A agent dispatches a genuinely fresh A worker through the normal server path and completes a deterministic turn after public selection changes to B.
- An existing session, including its reconnect path, does not silently switch generations; an eligible new session reaches B without terminating A.
- Missing, redirected, or replaced A fails safely rather than falling back to B. Reuse existing tests where they already establish this.
- Mixed CLI/agent versions connect when compatible or produce a clear incompatibility error. Installation of B does not falsely imply activation of B.
- If newly introduced Cercano startup/admission state can be interrupted, cover its smallest concrete recovery case. Do not create a broad failure matrix or new recovery infrastructure speculatively.

Use normal focused tests and race coverage where concurrency changes warrant it. Do not duplicate passing tests or require a full end-to-end suite for every internal edit.

### 3. One native A-to-B rehearsal

In the existing disposable macOS guest, build identifiable current-code A/B versioned fixtures containing the worker-selection fix. Do not invent a source change just to claim changed-source coverage. Record source revision, artifact hashes, commands, assertions, and teardown results.

Start an ordinary A agent and conversation, upgrade externally through Homebrew with the approved cleanup exclusion effective, and exercise automatic and explicit cleanup. Then have A itself complete a turn through a genuinely fresh A worker, verifying executable identity rather than warm-worker reuse or socket readiness alone. Verify a new eligible session reaches B and the A conversation can continue. Keep synthetic state isolated and check it survives the rehearsal.

Reuse previous package lifecycle and command-ownership evidence unless relevant packaging changes invalidate it. The rehearsal is not a Homebrew transaction test suite, a compatibility claim for arbitrary releases, or full release qualification.

### 4. Local-runtime safety boundary

Do not defer the dependency-safety question entirely to human testing. Determine whether the actual supported llama executable, libraries, backends, resources, and model paths remain usable for later work by an idle-but-active session across relevant external upgrades and cleanup. Agent-keg retention alone is not proof.

Prefer existing native-manager and Cercano mechanisms. Implement only concrete protection and compatibility changes necessary for the supported boundary. Preserve the approved rule that incompatible shared-runtime activation waits for dependent sessions to finish; no forced cancellation or required side-by-side incompatible model runtime.

If that boundary cannot yet be guaranteed, document the exact limitation and obtain explicit acceptance before describing the result as supported session-safe local upgrades. Do not silently substitute a cloud-only test, claim safety from one successful inference, or expand this effort into a custom runtime installer. A consequential unresolved protection design requires a focused decision before implementation.

### 5. Human acceptance checklist and handoff

Supply a short, reproducible checklist with expected outcomes and failure-report instructions covering:

- Normal conversations before and after upgrade, including a long-running/in-flight turn.
- Later turns and delegated work in the existing A conversation.
- Disconnect/reconnect behavior and a new B conversation while A remains usable.
- Local inference, including later use after an idle interval, within the declared supported runtime boundary.
- Clear installed/session version information and understandable deferred-activation or incompatibility messages.

Record human outcomes separately from automated and native-rehearsal evidence. A checklist is not a passing test result. Validate the next real changed-source release transition with humans using actual release artifacts; do not make an artificial changed-source fixture an engineering closeout requirement.

Check whether the historical monolithic package was actually distributed to users requiring migration support. If yes, include that specific upgrade and its restart-hook behavior in the native or human checklist and resolve any unsafe behavior before claiming supported migration. If not, omit historical migration work. Do not build a matrix of hypothetical old packages.

## Responsibility boundaries and anti-overengineering rules

Homebrew owns package download, verification, extraction, installation transactions, and package-operation recovery. No parallel transaction journal, custom updater, package rollback/retry engine, manager-owned file rewriting, manual link switching, or Homebrew fault-injection suite is authorized.

At the manager boundary, test only Cercano-visible compatibility, session safety, and truthful reporting. Use bounded fixtures for failed/unavailable package outcomes when relevant. Do not test Homebrew disk exhaustion, extraction interruption, or permission handling. Test Cercano's own write/recovery failures only where introduced changes create a concrete risk.

Keep the approved independent CLI/agent packages and convenience package. Keep persistent HOMEBREW_NO_CLEANUP_FORMULAE=cercano-agent retention; automatic old-keg retirement remains deferred. Verify the setting in the actual test invocation context rather than assuming the existing synthetic XDG setting applies globally. Do not silently configure other users or the host. Explicit uninstall, deliberate policy bypass, and direct deletion are outside continuity guarantees; selected-generation loss must still fail safely.

Every code change or test must close a named remaining gap. Prefer existing mechanisms and reuse valid evidence. No generalized update orchestration, exhaustive failure combinations, speculative hardening, or unrelated storage/credential redesign.

Signed/notarized artifacts, supported architecture/macOS-version coverage, distribution trust, and package publication remain in the existing release qualification process. Hand off a final-package smoke checklist; do not build a second release pipeline or make its credentials/runners prerequisites to finishing this engineering slice. Engineering completion is not release approval.

## Execution safety

Native package work is confined to the existing disposable guest or separately authorized disposable environments. Use synthetic state and controlled deterministic services by default. Enumerate model acquisition, network, disk/memory needs, and credentials before relevant execution and seek authorization where not already granted. Never use the developer's live agent, model state, credentials, or Homebrew installation. Stop only owned test processes; preserve evidence and explicitly record residual guest configuration and packages.

## Completion and non-goals

Engineering closeout requires the minimal implementation, focused critical-invariant tests, the native rehearsal, an explicit local-runtime support boundary, and the human checklist. Report unresolved limitations and pending human/release checks plainly. Do not label the feature fully accepted merely because the engineering deliverables are complete.

Out of scope: Linux/Windows completion, automatic keg retirement, arbitrary cross-version compatibility, deliberate-deletion guarantees, custom package/runtime installers, broad failure matrices, duplicate release infrastructure, unapproved downloads/credentials, production publication, and unauthorized push or landing.

The package topology, native-manager ownership, agent retention, and deferred incompatible activation policies are settled. Escalate only a concrete consequential decision discovered during implementation; do not reopen those policies or hide unresolved safety behind additional tests.
