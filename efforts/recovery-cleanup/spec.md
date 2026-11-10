# Recovery Cleanup

## Status

Proposed specification awaiting user sign-off. The user approved rejecting legacy client stop confirmations with upgrade guidance and deferring wire-envelope migration. No cleanup implementation is authorized by this document.

## Problem and motivation

The landed old-host-compatible recovery implementation retains duplicate destination-side recovery and consent orchestration alongside the contained-source coordinator. The audit also identified unused helpers, an unused takeover primitive, an orphan fixture runner that can select no tests, misleading internal names, and stale documentation. Deferred fallback currently obscures which ordinary failures are eligible for contained recovery.

The audit is a source-review baseline, not proof that every candidate remains unused at execution time. Recheck references and behavior before deletion. This effort simplifies existing behavior without weakening recovery safeguards or claiming to repair a particular live conversation.

## Goals

Keep one destination-side contained-recovery execution path and one fresh, explicit consent model. Preserve ordinary acquisition, cooperative old-host transfer, and exact reservation claims before contained fallback. Reject legacy client stop confirmations clearly with upgrade guidance rather than executing the older stop workflow.

Remove genuinely unused code and orphan runnable scaffolding. Preserve useful unique safety assertions by moving them to active APIs where appropriate. Retained acceptance runners must reject zero intended test execution.

Make fallback eligibility explicit and auditable, retaining original failure context in diagnostics. Authentication failures, cancellation, ambiguous or failed metadata reads, and uncertain persistence must not silently become stop authorization. Characterize eligible ordinary failures before restructuring their handling.

Consolidate the production adapter around continuous contained use, retaining native absence verification as authority. Improve internal naming and current documentation without migrating the wire representation.

## Constraints

Preserve source authentication, exact selected identities and current ownership epochs, stable conversation locks, transactional reservation checks, launch exclusion through reassignment and reload, uncertain-outcome refusals, and fresh requester-bound consent. Historical receipts never establish current ownership or readiness.

Preserve compatibility with supported older source hosts through existing cooperative controls. This is separate from compatibility with older clients requesting the superseded destination-side stop workflow. Such requests must fail explicitly before stopping or reassigning anything.

Preserve the active LifecycleOriginalRecovery wire envelope and field numbers in this effort. Obsolete legacy/takeover modes remain rejected. Internal renaming must not silently alter the protocol contract.

Native containment remains bounded to the supported same-user macOS prepared-generation host/worker model; this effort does not extend platform support or fence arbitrary SQL writers. Cross-compilation is not runtime qualification. Historical fixture evidence does not certify the exact dirty incident binary. External effects are not rolled back or automatically replayed.

Do not delete untracked evidence, local temporary artifacts, unrelated worktrees, or historical verification records. Do not interact with live hosts, user databases, credentials, installations, or VMs as part of this cleanup. Push, deployment, and live recovery require separate authorization.

## Decisions

### Legacy client confirmations

The user approved explicit rejection with upgrade guidance. Remove the parallel legacy execution path while retaining only the compatibility handling needed to refuse old requests safely. Old-host cooperation remains supported. This avoids translating stale or weaker consent into authorization under the new model.

| Axis | Reject legacy confirmations clearly — chosen | Translate into new consent flow — not chosen |
|---|---|---|
| Complexity | Delete parallel execution; retain bounded rejection and compatibility tests | Add adapter and renewed-consent handling with mixed-client journeys |
| Risk | Older clients must upgrade for recovery; failure is explicit | Old clients may not present or return required consent; unsupported cases still reject |
| Outcome | One execution path and consent model | One execution path with additional compatibility machinery |
| Side effects | Smaller maintenance surface; clear upgrade guidance required | Larger protocol and test surface |

### Wire envelope

The user approved deferring wire migration. Preserve the current envelope while renaming internal helpers and documenting the mapping. A later protocol migration can address historical names and obsolete fields with explicit mixed-version behavior.

| Axis | Preserve envelope — chosen | Migrate now — not chosen |
|---|---|---|
| Complexity | Internal names and documentation only; no schema regeneration | Protocol, bindings, consumers, and mixed-version tests |
| Risk | Historical naming remains but transport stays stable | Older clients may not understand new responses |
| Outcome | Isolated execution-path consolidation | Immediate alignment of wire terminology |
| Side effects | Documented follow-up remains | Larger review and qualification scope |

## Proposed delivery order

First eliminate misleading zero-test fixture success and prove deletion candidates are unused. Remove the unused reaffirmation helper and takeover primitive in small checkpoints, preserving active publication barriers and useful safety tests.

Next consolidate legacy consent and execution, with cooperative-first and compatibility regressions. Then replace deferred fallback with explicit classified outcomes and specified failure behavior. Simplify the adapter only after removing its legacy consumers; retain final native verification and continuous exclusion.

Finally consolidate documentation and rerun final historical acceptance and focused integration coverage. Each solved unit receives a scoped checkpoint. These are priorities, not the execution checklist; the plan follows specification approval.

## Acceptance

Reference searches establish removed APIs and scaffolding have no remaining consumers. Retained fixture runners distinguish executed tests from a successful zero-test command.

Tests establish zero shutdown calls for ordinary cooperative resume and valid exact reservation claims, adapter-less cooperation, preserved operation identity on ready retries, and explicit side-effect-free rejection of legacy confirmations. Consent tests cover decline, expiry, replay, requester and identity changes, and ownership/reservation changes before stop.

Fallback tests explicitly cover authentication mismatch, cancellation, uncertain publication or persistence, malformed journals, acceptance cleanup failure, source response loss, and receipt lookup errors. Original error context remains available without disclosing credentials or receipts.

Adapter tests cover attribution errors, surviving generation processes, continuous launch exclusion, response loss, bounded drain failures, and refusal when absence is uncertain. No replacement of native verification with a permissive test hook is acceptable.

Run focused normal and race tests for ownership, server recovery, process controls, agentclient, and CLI recovery; compile both modules. Repeat historical owned/drain, reserved, and blocked-timeout acceptance on the final candidate, including exact history-prefix preservation, exactly two added turn rows, final-response and provider-request cardinality, and owned-process teardown. Record limitations and actual selected tests rather than implying a full-suite or deployed acceptance result.

## Non-goals

No new recovery protocol, generic workflow framework, wire-envelope migration, cross-platform runtime support, live incident intervention, automatic replay of interrupted actions, or wholesale deletion of historical evidence. This effort is not authorization to push or deploy.
