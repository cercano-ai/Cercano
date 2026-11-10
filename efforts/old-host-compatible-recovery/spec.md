# Old-host-compatible conversation recovery

## Problem

A new Cercano build must be able to recover a conversation owned by an older host that lacks newly introduced retry and takeover RPCs. The accumulated original-operation discovery, exact-retry, and cooperative-takeover protocols have not met that requirement. Multiple failed destination journals can block discovery, and probing a new RPC on an old owner can abort reopen before a compatible recovery route is offered.

The prior audit is source-inspection evidence, not proof of deployed-binary behavior or successful live recovery. This effort must not equate fixture success between current implementations with compatibility with the incident's old implementation.

## Goals

Provide one destination-coordinated recovery flow. Prefer safe cooperation through controls supported by the old host. When cooperation cannot complete, offer explicit interruption of the exact old owning host, explain effects on its other conversations and tasks, establish that relevant old writers cannot continue or relaunch, recover durable ownership, reload committed history, and complete a persisted turn on the selected new host.

Multiple failed destination attempts must not permanently prevent independently authorized, death-proven recovery. Historical handover results must not be mistaken for current ownership. Restart-lost in-memory receipts or closures must not be fabricated from journal contents.

Remove redundant recovery orchestration and misleading UI/documentation while preserving reusable safety fixes and regression evidence. Recovery must not require the old process to implement newly added RPCs.

## Approved behavior

The user reaffirmed on 2026-11-09 that whole-old-host interruption after explicit disclosure was already approved. This is not an open design decision. Force reopen may stop the exact owning host when safe cooperative recovery cannot finish, warning that other conversations and tasks hosted there may be interrupted. Implementation approval does not authorize executing recovery against the user's live conversation in this planning effort.

The new host owns coordination. Durable current ownership and authenticated process attribution govern recovery; journal enumeration is evidence discovery, not ownership authority. Unsupported optional RPCs must not make the compatible fallback unreachable. No mutating RPC is used merely to discover which private source reservation exists.

## Safety contract

Before host interruption, bind fresh consent to the exact source identity, conversation epoch, selected destination, and disclosed scope. Revalidate that identity and establish source launch exclusion. Stop only the attributed source and relevant owned workers through reviewed process-control mechanisms; never signal a bare historical PID or broadly terminate Cercano processes. Verify death and relevant writer containment before durable reassignment. If containment or attribution is uncertain, report the specific blocker rather than claiming success.

Participating current-store ownership fencing does not retrofit protection onto arbitrary older SQL connections. Neither changing a database row nor obtaining an advisory lock alone proves an old process cannot write. Verified death also does not prove all earlier operations succeeded: preserve committed results and explicitly report interrupted or uncertain work. Do not automatically replay interrupted tasks or external actions. Normal durable completion-notification scheduling must be distinguished from replay and tested/documented accurately.

Recovery must lock and recheck exact durable ownership and affected descendant state. An owned row and a reserved handover require distinct checks. Reserved-state recovery must serialize against legitimate late claims and reject stale source epochs. A new owner already claimed by another destination must never be overwritten under consent for an earlier source epoch. Existing dead-owner recovery's rejection of reserved rows is a known implementation gap, not permission to bypass validation.

Unknown transaction outcomes remain blocking until reconciled. Retry must preserve operation identity and distinguish historical success from current ready ownership. Cancellation, timeout, crash, and response loss must not silently generate a competing operation or grant automatic renewed consent.

## Cleanup scope

Replace the mandatory original-journal uniqueness detour in server resume discovery and its CLI recovery orchestration. Consolidate normal reopen and force recovery into one coordinator rather than maintaining parallel user journeys.

Remove redundant retry/takeover handlers, client wrappers, prompts, messages, mocks and documentation after tracing all consumers. Preserve any demonstrably necessary primitive behind the single coordinator rather than maintaining a separate repair product. Removed protobuf numbers and names must be reserved, never reused; existing old control compatibility needed by the replacement must be preserved.

Preserve or port exact source/epoch validation, current-owner claim validation, ownership transaction locks and write fencing, admission and persistence accounting, authenticated destination identity, publication durability checks, and useful regressions. Do not wholesale revert safety changes along with superseded protocols. Historical verification documents remain historical evidence with corrected scope, not proof of the replacement.

Likely affected areas are source/proto/agent.proto; source/server/internal/server/{resume,resume_discovery,retry_transfer,takeover,takeover_confirmation,lifecycle_controls,dead_owner}.go; source/server/internal/conversation ownership/recovery code; source/server/internal/broker admission code; source/server/pkg/agentclient recovery clients; command-level process attribution/stop wiring; CLI reopen/recovery UI; generated protocol consumers and mocks; and recovery documentation. Exact deletion paths require dependency tracing before removal.

## Acceptance

Use an isolated actual old implementation without new retry/takeover APIs, not just current handlers returning Unimplemented. Record old-binary provenance and limitations; dirty manifest commits alone cannot reproduce the incident binary.

The primary journey starts with the old host owning a conversation, a retained failed transfer, and multiple failed destination journals. A new build requests recovery, correctly handles unavailable new RPCs, obtains explicit consent where required, safely interrupts the attributed old host when cooperation cannot finish, acquires fresh ownership, and completes one exact persisted turn on the new host. This is the acceptance target, not merely receipt validation, process survival, or current-to-current fixture success.

Cover cooperative success, declined interruption without mutation, sibling impact disclosure, pending and uncertain persistence, source restart-lost authority, owned and reserved rows, late claims, stale approval/PID/epoch, surviving workers, relaunch attempts, response loss, idempotent retry, and absence of automatic interrupted-action replay. Assert old writers cannot continue and unrelated hosts/data remain untouched. Tests must establish nonzero selected coverage; normal and race checks supplement, not replace, the old-process integration journey.

## Non-goals and execution boundary

No live conversation recovery, production restart, manual journal deletion, direct production ownership edits, publication, push, or unrelated Windows work is authorized by this specification. Do not implement another custom updater or require upgrading the old owner. Compatibility limits must be explicit and supported by evidence. Do not claim all historical binaries are recoverable until their store and process contracts are established.

## Status

Draft specification records the approved flow and cleanup scope. No implementation or live mutation has been performed for this effort. Execution plan follows specification sign-off.
