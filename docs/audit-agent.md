# Agent documentation audit — 2026-10-22

## Scope and completion boundary

Scope: all 83 files under `docs/agent/` plus `docs/cloud-routing.md` (84 total). The original pass edited only that scope and this report. This cumulative ledger now incorporates the [eight-document specialist audit](audit-agent-specialists.md) and
the [11-document feature audit](audit-agent-features.md); the original pass delivered concrete semantic corrections and historical boundaries; **it does not establish exhaustive line-by-line implementation verification of every maintained claim**. The ledger below distinguishes targeted comparisons from historical classification and completed bounded full-file audits.

## Corrections and source evidence

- Backend quirks now describe the actual three-field descriptor and Mistral handling, not removed transport retries: `source/server/internal/llm/openai/quirks.go`; retry policy: `source/server/internal/inference/resilience/resilience.go`.
- Provider support includes messages, Chat Completions, Responses and Bedrock, with subscription token-source requirements and vision gating: `source/server/internal/cloudfactory/factory.go`.
- Responses storage language no longer guarantees zero provider retention or identical model quality; request construction uses `Store: false` and encrypted reasoning inclusion: `source/server/internal/llm/responses/client.go`. Shared transport retry proposal replaced with current resilience ownership.
- Public delegation guide now documents durable acceptance, not synchronous completion: `source/server/internal/taskrun/manager.go`; manager execution detaches launching-turn cancellation and preserves host lifetime.
- Dispatch default corrected to Secondary/Standard; compaction and watchdog troubleshooting corrected to task-aware routing: `source/server/pkg/config/routing.go`, `source/server/internal/loopcompact/wiring.go`.
- Isolation architecture now identifies worker as production default and documents idle timeout: `source/server/pkg/config/config.go`.
- Compaction fidelity format includes FINDINGS; task reference is distinct from user intent: `source/server/internal/compaction/working_memory.go`; parser coverage in `summarizer_test.go:137-158`.
- Trajectory export has an implementation-status section: `source/server/internal/server/trajectory_export.go` implements server-owned export and streamed status. No external ATIF interoperability certification is implied.
- Historical plans/designs and dated provider observations have explicit boundaries and links to current guides. Locus designs additionally identify superseding task routing. Historical steps and embedded approvals are not execution instructions.
- Self-development tracing build example now runs from the server module; historical unchecked plan boxes are not described as a reliable remaining backlog. Cloud routing distinguishes code behavior from external provider verification.

## Checks actually run

From `source/server`:

```text
go test ./pkg/config ./internal/taskrun ./internal/inference/resilience ./internal/llm/openai ./internal/loopcompact ./internal/compaction ./internal/trajectory
```

PASS (seven packages; five cached, config and trajectory executed). No live-provider flags, paid inference, credential sign-in, local-model smoke, full-suite build, PTY/UI tests or external links were exercised. Existing dated live results in provider documents remain historical evidence, not new verification.

## Follow-up coverage

The [specialist report](audit-agent-specialists.md) completes bounded source audits
of compaction routing/working memory, dispatch diagnostics/reasoning, independent
tasks, Mistral, protobuf regeneration and maintained vision guidance. It records
20 server-package tests, CLI UI tests and matching temporary protobuf outputs;
these were run by that task, not rerun for this reconciliation. Vision is no
longer classified as historical-only. Independent-review protobuf fixes are
included there; retired cloud-model, enterprise catalog discovery and the
unfinished workflow corrections are recorded in the [reference audit](audit-reference.md).

The [feature report](audit-agent-features.md) completes source audits of all 11
feature documents, including the index, with focused deterministic tests. These
are documentation source checks, not live-provider or full runtime certification.
See the [central report](publicity-freshness-audit.md) for CLI/reference coverage
and publication and implementation advisories.

The [core-guide follow-up](audit-core-guides.md) is **PARTIAL** across all five
guides: it publishes completed targeted corrections, not a completed five-file
semantic audit. Self-dev and cloud routing receive no new full-file certification.
Its focused test result is attributed to that audit; publication review spot-checks
do not close the remaining coverage.

## Coverage limits and acceptance boundaries

- Full-file feature and specialist source audits are complete within their stated
  scopes. Other maintained agent entries below still record **targeted** source
  comparisons, not exhaustive claim-by-claim review; those are the remaining
  local semantic coverage limits, not a pending feature-group audit.
- Historical banners deliberately preserve original bodies rather than
  reconciling every old API/status sentence as current operating instructions.
- Account identity refresh, UI draft persistence, task notification presentation
  and permission-mode end-to-end behavior were not acceptance-tested. Image/cache
  and task behavior have specialist source/test coverage. These unrun acceptance
  checks do not prevent completion of bounded documentation source audits.
- Mistral live capability/account assertions require external validation.

## Exact file coverage ledger

| File | Cumulative coverage (original pass plus reported follow-up) |
| --- | --- |
| `docs/agent/README.md` | **PARTIAL** core-guide follow-up; targeted comparisons only; see [remaining coverage](audit-core-guides.md) |
| `docs/agent/agent-isolation/architecture.md` | **PARTIAL** core-guide follow-up; targeted comparisons only; see [remaining coverage](audit-core-guides.md) |
| `docs/agent/agent-isolation/design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/agent-isolation/plan-phase2.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/agent-isolation/plan-phase3.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/agent-isolation/plan-phase4.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/agent-isolation/plan-phase5.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/agent-isolation/plan-phase6.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/agent-isolation/plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/cloud-bedrock-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/cloud-bedrock.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/cloud-failover-audit.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/cloud-openai-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/cloud-openai.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/cloud-profiles-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/cloud-profiles.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/cloud-responses-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/cloud-responses.md` | Targeted semantic correction / source comparison; historical boundary also added |
| `docs/agent/compaction-decision-fidelity.md` | **PARTIAL** core-guide follow-up; targeted comparisons only; see [remaining coverage](audit-core-guides.md) |
| `docs/agent/compaction-routing.md` | Completed bounded specialist source audit; see [evidence and validation limits](audit-agent-specialists.md) |
| `docs/agent/compaction-working-memory.md` | Completed bounded specialist source audit; see [evidence and validation limits](audit-agent-specialists.md) |
| `docs/agent/context-management/compaction/compaction-2a-algorithms-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2a-algorithms-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2a-foundation-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2b1-livewiring-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2b1a-engine-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2b1b-wiring-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2b2-retention-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2b2-retention-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2b3-visibility-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2b3a-data-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2b3b-footer-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-2b3c-cview-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-bakeoff-findings.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-lifecycle-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-nonblocking-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/compaction/compaction-nonblocking-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/context-manipulation/chatpane-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/context-manipulation/chatpane-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/context-manipulation/design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/context-manipulation/plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/context-manipulation/rework-prompt-bar-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/context-manipulation/rework-prompt-bar-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/context-view/design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/context-view/expand-turns-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/context-view/expand-turns-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/context-view/plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/meter-wiring/design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/meter-wiring/plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/recap-title/recap-title-design.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/context-management/recap-title/recap-title-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/dispatch-diagnostic-trace.md` | Completed bounded specialist source audit; see [evidence and validation limits](audit-agent-specialists.md) |
| `docs/agent/dispatch-reasoning-capture.md` | Completed bounded specialist source audit; see [evidence and validation limits](audit-agent-specialists.md) |
| `docs/agent/features/README.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/features/advanced-metrics.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/features/advanced-routing.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/features/advanced-terminal-ui.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/features/automatic-session-retention.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/features/built-in-delegation.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/features/client-server-architecture.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/features/context-management.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/features/integrated-local-runtime.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/features/openai-providers.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/features/powerful-agent-workflows.md` | Completed bounded full-file feature source audit; see [evidence and validation limits](audit-agent-features.md) |
| `docs/agent/independent-tasks.md` | Completed bounded specialist source audit; see [evidence and validation limits](audit-agent-specialists.md) |
| `docs/agent/llm-backend-notes.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/locus-coproc-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/locus-coproc.md` | Targeted semantic correction / source comparison; historical boundary also added |
| `docs/agent/locus-mode-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/locus-mode.md` | Targeted semantic correction / source comparison; historical boundary also added |
| `docs/agent/meridian-opencode-route.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/mistral.md` | Completed bounded specialist source audit; see [evidence and validation limits](audit-agent-specialists.md) |
| `docs/agent/per-backend-quirks-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/per-backend-quirks.md` | Targeted semantic correction / source comparison |
| `docs/agent/proto-regen.md` | Completed bounded specialist source audit; see [evidence and validation limits](audit-agent-specialists.md) |
| `docs/agent/runtime-agnostic-model-lifecycle-plan.md` | Historical boundary reviewed/added; original design body retained (not fully revalidated) |
| `docs/agent/self-dev.md` | **PARTIAL** core-guide follow-up; targeted comparisons only; see [remaining coverage](audit-core-guides.md) |
| `docs/agent/sub_agents/README.md` | Targeted semantic correction / source comparison; historical boundary also added |
| `docs/agent/trajectory-export.md` | Targeted semantic correction / source comparison |
| `docs/agent/vision-input.md` | Completed bounded specialist source audit; see [evidence and validation limits](audit-agent-specialists.md); maintained guidance plus historical design |
| `docs/cloud-routing.md` | **PARTIAL** core-guide follow-up; targeted comparisons only; see [remaining coverage](audit-core-guides.md) |

## Local Markdown target scan

Scanned inline Markdown links in the 84 scoped files (filesystem targets only; anchors and external URLs excluded). Missing targets: 0.
