# Publicity documentation freshness audit

Baseline: `d93f490490f6b46693d6493868ae4d85871f2396`. Branch: `docs/publicity-refresh`.

## Scope and coverage status

**Cumulative source-backed documentation audit, not a release sign-off or
all-claims certification.** The baseline inventory covers 521 tracked
Markdown/MDX/RST/AsciiDoc and website TSX/HTML files. Inventory inclusion and
bounded link scans are not sentence-by-sentence source verification. Subsequent
reports supersede initial inventory-only labels for their explicitly scoped files:

- [Agent audit and cumulative ledger](audit-agent.md): 83 agent documents plus
  cloud routing; targeted source corrections and historical boundaries.
- [CLI audit](audit-cli.md): three current guides, 34 nested historical records
  and the preserved roadmap (38 documents after the update); registered command
  families, configuration, model selection and platform boundaries source-checked.
- [Reference/distribution audit](audit-reference.md): reference, enterprise,
  release/platform and website guidance, canonical skills and generated mirrors,
  plus explicit current-reference versus historical feature dispositions.
- [Agent specialist audit](audit-agent-specialists.md): eight formerly outstanding
  specialist guides source-audited, with deterministic tests and protobuf output
  comparison; vision now has maintained guidance above its historical design.
- [Agent feature audit](audit-agent-features.md): all 11 feature documents,
  including the index, completed bounded full-file source audits and focused tests.

- [Core-specialist follow-up](audit-core-specialists.md): six additional guides'
  maintained repo-verifiable claims reviewed; current implementation limits
  supersede historical Responses/locus/sub-agent/export proposals. No external
  provider or ATIF certification. This does not close the partial core-guide audit.

- [Core-guide follow-up](audit-core-guides.md): five-guide closure remains
  **PARTIAL**. Decision-fidelity and [self-dev](audit-core-self-dev.md)
  maintained-claim comparisons are complete within their stated source-only
  boundaries; self-dev publication did not independently revalidate every claim.
  [README startup/headless](audit-core-readme-startup.md) and the
  [permission section](audit-core-readme-permissions.md) have bounded reports.
  README and cloud routing retain explicit remaining coverage. Isolation now has
  [maintained-claim source closure](audit-core-isolation-closure.md), not
  complete-file or live acceptance certification.

Progression: the initial targeted correction/link-scan pass was partial; the
follow-up fixed the classify example and 11 missing relative targets; scoped
CLI, reference, agent and specialist reports then expanded coverage. Historical
bodies remain records, not current acceptance checklists. Counts overlap and
must not be summed as independently verified files. Remaining local gaps and
external publication checks are separated below.

## Corrections and source evidence

| Surface | Correction / checked evidence |
| --- | --- |
| Website launch / server module README | `cercano-cli` is the client; bare `cercano` starts server. `source/server/cmd/cercano/main.go` switch and server-mode comment; both module Makefiles and go.mod files. Replaced obsolete module setup/build layout and mandatory Ollama. |
| Routing and self-dev | Secondary default dispatch/investigation/implementation/review/research quality is Standard; added Compaction row (Secondary/Light). `source/server/pkg/config/routing.go` taskDefinitions. |
| Runtime feature guide | Default `llama_server`, optional `mistralrs` and Ollama; `source/server/pkg/config/config.go` Default and MistralRSConfig; `internal/localruntime/` is the runtime implementation location. |
| Agent setup and permissions | Removed legacy cloud_model instructions and automatic Ollama-only claim; ordinary built-in W/X permissions agree with `internal/agent/permissions.go` and tests. MCP and human-handoff gates remain exceptions. |
| Contributor instructions | Removed unconditional local/free research promises; route selection is configurable, not guaranteed by tool name. |
| Docs landing | Added current user navigation and historical-record distinction. |
| Embedded inference / vision / enterprise status / technology overview | Added historical context, not retrospective changes to original proposals. Image inheritance exists in `internal/server/image_inheritance_test.go`; native image inspection in `internal/capabilities/builtins/inspect_image.go`. Enterprise external PR state not revalidated. |
| Native inference | Removed obsolete “no commit yet”; qualified W-tier confirmation and provider availability. Inspected `internal/capabilities/builtins/deepinfra_infer.go` constraints. |
| Task lifecycle | Read `docs/agent/independent-tasks.md`; taskrun package tests passed. Not an exhaustive audit of transport/privacy claims. |
| Multi-account failover | Read current cloud routing/agent guidance; inspected `internal/inference/profilechain/chain.go`; package tests passed (package-level coverage only). Not live provider validation. |
| Session model overrides | Read session-model guide; inspected permission boundary; chatroute and CLI slash tests passed. Not a full UI end-to-end check. |
| Installation / releases | Read root installation/platform instructions and `release/homebrew/README.md`; no package published/downloaded/installed or signature/notarization check. |

All source paths above are under `source/server/` unless otherwise qualified.

## Verification performed

- `go test ./pkg/config ./internal/inference/profilechain ./internal/taskrun ./internal/chatroute`: passed (server module).
- `go test ./internal/agent -run 'TestGateDecision|TestPermissionStore' -count=1`: passed.
- `go test ./internal/slash`: passed (CLI module).
- Website `npm ci --ignore-scripts`: succeeded; audit reported 29 dependency vulnerabilities (2 low, 2 moderate, 24 high, 1 critical). No dependency changes made.
- Website `npm test`: build and all 3 rendered HTML tests passed.
- Website `npm run lint`: fails on existing `setTheme(active)` in `app/page.tsx:121` (`react-hooks/set-state-in-effect`), outside the changed copy. No implementation fix attempted.
- `git diff --check`: passed before final report generation.
- Local inline Markdown links: scanned tracked Markdown outside fenced blocks,
  resolving relative paths against each document, comparing against baseline
  tracked paths. Does not validate anchors, reference-style links, images,
  external URLs or templated paths. 11 preexisting missing targets remained in the initial pass;
  the follow-up below resolves them.
- No full product integration suite, agent installation/restart, or live paid
  inference. No pushes or landing in the initial pass; branch publication of the
  six-guide follow-up is recorded separately below.

## Independent review

The initial read-only review covered only the targeted corrections. Subsequent
independent-review findings were corrected by the responsible documentation tasks:
retired `cloud_model` examples now explain rejection and current routing;
enterprise catalog discovery describes built-in tools, protocols and managed
assignments rather than personal/repository directory scanning; protobuf guidance
uses the canonical script with a verified temporary-output comparison; the
unfinished workflow reference now documents skill regeneration and module-specific
build/test commands. See the [reference report](audit-reference.md) and
[specialist report](audit-agent-specialists.md) for evidence and limits. Review
is not certification of all baseline inventory entries.

## Remaining local documentation coverage limits

- No feature-group source audit remains pending. Agent ledger entries labeled
  **Targeted semantic correction / source comparison** or **PARTIAL** have not received the same
  full-file claim-by-claim audit as the feature and specialist groups. The exact
  paths are retained in the [agent ledger](audit-agent.md#exact-file-coverage-ledger);
  this pass does not claim all repository documentation is exhaustively verified.
- Historical bodies remain deliberately unreconciled records, not current guides.
  Source paths in historical prose and templated links are not runnable contracts.
- CLI help/handler discrepancies for `/clear`, direct `/tool` confirmation and
  model-tier key shapes, optimistic permission chips, and MCP schema/empty-result
  legacy terminology remain **implementation advisories**, not missing doc edits.
  Current guides describe the handlers rather than repeat those stale strings.

## Unrun runtime acceptance (not documentation source-audit prerequisites)

Account identity refresh, UI draft persistence, task notification presentation,
permission-mode end-to-end behavior, interactive setup, PTY/UI acceptance, full
product integration, local-runtime hardware smoke and installation/restart were
not exercised. Specialist image/cache/task and feature source audits are complete
within their stated scopes; they do not certify deployed behavior. Full runtime
certification is not required to finish these bounded documentation audits.

## External publication validation still required

- Live release URLs/digests, Homebrew availability/install/upgrade, publisher
  signing/notarization, native platform support and live Pages configuration.
- Provider eligibility/terms, models, pricing, latency, cost savings and live
  Mistral capabilities; web search and external skill-host interoperability.
- Enterprise PR/acceptance/deployment status, organization login/Keychain UX and
  organization-specific security guarantees. Wire contracts are not deployment proof.
- No blanket privacy/offline or zero-cost guarantee follows from local inference,
  passing tests or generated skill parity; arbitrary network tools and credential
  exports require their own validation.

## Preexisting lint and dependency advisories

The initial website checks reported 29 dependency vulnerabilities (2 low,
2 moderate, 24 high, 1 critical) and a lint failure at `app/page.tsx:121`
(`react-hooks/set-state-in-effect`, `setTheme(active)`). These preexisting
advisories were not remediated; successful static-export tests do not clear them.
The verification results above and in linked reports are attributed to those
passes, not new tests run during this report reconciliation.

## Follow-up: skill example and preexisting local-link targets

Historical progression: this follow-up did not complete the maintained-document
semantic audit. CLI, specialist, enterprise, skill-mirror, release and platform
source comparisons were subsequently recorded in the linked reports above;
the current remaining gaps are listed separately, not inherited from this stage.

| Document | Disposition and source evidence |
| --- | --- |
| `docs/reference/agent-skills-guide.md` | Corrected classify example: configured Reconnaissance route rather than guaranteed local/Ollama inference; added `file_path`, precedence, and qualitative model confidence. Evidence: `source/server/internal/capabilities/builtins/coproc_classify.go:23-68`. Checked MCP skill list/get registration in `source/server/internal/mcp/server.go:369` and dynamic capability registration following `registerTools`. Removed universal host-discovery claims; external skill-host/spec compatibility not certified. Fixed repository skill link. |
| `docs/features/agent-capabilities/task-model/research/README.md` | Historical generated research, explicitly labeled and linked to existing `CAVEATS.md`. That record documents deliberate deletion of four off-topic findings and six reference artifacts. Preserved historical titles as pruned entries rather than inventing destinations or restoring deleted material. No retrospective factual rewrite of the synthesis. |

All 11 previously listed missing targets are now resolved: one corrected relative
path and ten intentionally pruned historical entries converted to non-links with
an explanation. Follow-up inline-link scan of 520 tracked Markdown files found zero missing
relative targets (outside triple-backtick fences; images, reference links,
anchors, external URLs, and templated paths excluded). This does not certify
anchors or external destinations. Website files were not changed.

## Tracked inventory

This is the **initial-pass baseline ledger**, retained for inventory provenance,
not a cumulative per-file completion table. `corrected` means targeted initial
corrections, not exhaustive verification. `inventory/link scan only` records that
initial stage; linked scoped reports above supersede it where they name a file.
Use the cumulative agent ledger for specialist/feature status, rather than
interpreting these historical labels as new outstanding work.

| Path | Classification | Coverage |
| --- | --- | --- |
| `.agents/skills/cercano-classify/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-config/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-deep-research/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-document/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-explain/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-extract/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-fetch/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-init/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-local/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-models/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-research/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-stats/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-submit-usage/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/cercano-summarize/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/compute-before-simulate/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/design-decisions/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/systematic-debugging/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/verification-strategy/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.agents/skills/worktree-first/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-classify/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-config/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-deep-research/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-document/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-explain/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-extract/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-fetch/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-init/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-local/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-models/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-research/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-stats/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-submit-usage/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/cercano-summarize/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/compute-before-simulate/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/design-decisions/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/systematic-debugging/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/verification-strategy/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.claude/skills/worktree-first/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `.superpowers/sdd/report-D.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `AGENTS.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `CLAUDE.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `README.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/README.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/agent/README.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/agent/agent-isolation/architecture.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/agent-isolation/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/agent-isolation/plan-phase2.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/agent-isolation/plan-phase3.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/agent-isolation/plan-phase4.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/agent-isolation/plan-phase5.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/agent-isolation/plan-phase6.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/agent-isolation/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/cloud-bedrock-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/cloud-bedrock.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/cloud-failover-audit.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/cloud-openai-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/cloud-openai.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/cloud-profiles-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/cloud-profiles.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/cloud-responses-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/cloud-responses.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/compaction-decision-fidelity.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/compaction-routing.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/compaction-working-memory.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2a-algorithms-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2a-algorithms-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2a-foundation-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2b1-livewiring-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2b1a-engine-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2b1b-wiring-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2b2-retention-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2b2-retention-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2b3-visibility-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2b3a-data-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2b3b-footer-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-2b3c-cview-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-bakeoff-findings.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-lifecycle-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-nonblocking-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/compaction/compaction-nonblocking-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/context-manipulation/chatpane-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/context-manipulation/chatpane-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/context-manipulation/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/context-manipulation/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/context-manipulation/rework-prompt-bar-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/context-manipulation/rework-prompt-bar-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/context-view/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/context-view/expand-turns-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/context-view/expand-turns-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/context-view/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/meter-wiring/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/meter-wiring/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/context-management/recap-title/recap-title-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/context-management/recap-title/recap-title-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/dispatch-diagnostic-trace.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/dispatch-reasoning-capture.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/features/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/features/advanced-metrics.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/features/advanced-routing.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/features/advanced-terminal-ui.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/features/automatic-session-retention.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/features/built-in-delegation.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/features/client-server-architecture.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/features/context-management.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/features/integrated-local-runtime.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/agent/features/openai-providers.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/features/powerful-agent-workflows.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/independent-tasks.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/llm-backend-notes.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/locus-coproc-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/locus-coproc.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/locus-mode-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/locus-mode.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/meridian-opencode-route.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/mistral.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/per-backend-quirks-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/per-backend-quirks.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/proto-regen.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/runtime-agnostic-model-lifecycle-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/agent/self-dev.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/agent/sub_agents/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/trajectory-export.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/agent/vision-input.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/archive/_migration_audit.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/archive/_reorg_audit.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/archive/dispatch.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-07-04-meridian-resume-replay-report.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-07-04-user-message-tear.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-07-06-malformed-tool-input.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-07-09-worker-turn-persistence.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-09-17-worker-mcp-permission-liveness.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-09-20-worker-dispatch-compaction.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-09-20-worker-log-forwarding.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-09-22-dispatch-correctness.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-09-22-dispatch-prompt-contract.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-09-23-glm-reasoning-effort-pilot.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/2026-10-18-cloud-subagent-compaction-task-reference.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/deepinfra-dispatch-followups.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/bugs/reasoning-continuation-diagnostic.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/cloud-routing.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/co-processor/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/co-processor/legacy-readme.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/decisions/autonomous_2026-06-24.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/decisions/autonomous_2026-07-13.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/enterprise-connection.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/enterprise-implementation-status.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/enterprise-managed-routing.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/enterprise-runtime-enforcement.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/enterprise-shared-skills.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/adk-integration/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/capability-architecture/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/capability-architecture/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/dispatch-engine/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/dispatch-engine/implementation-notes.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/dispatch-engine/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/git-workflows/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/git-workflows/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/01_agent-to-human-handoff-patterns-designing-escalation-that.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/02_getting-started-with-conductor-for-gemini-cli-kdnuggets.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/03_32-claude-code-tricks-that-actually-change-how-you.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/04_plan-mode-in-claude-code-think-before-you-codewithmukesh.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/05_how-to-use-claude-code-as-a-product-manager-2026.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/06_what-is-plan-mode-in-claude-code-claudelog.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/07_planner-executor-evaluator-loop-architecture.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/08_how-to-use-aider-in-2026-setup-architect-mode-git-wor.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/09_coding-tool-reviews-aider-s-architect-mode-and-benchmarks.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/10_aider-s-architect-mode-when-the-slower-more-expensive.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/11_step-by-step-guide-refactoring-a-large-rust-codebase-with.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/12_aider-s-architect-mode-plan-before-you-code-ai-stack-today.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/13_exec-plan-ai-first-ssot.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/14_human-in-the-loop-ai-agents-when-approval-gates-matter.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/15_claude-code-plan-mode-design-review-first-datacamp.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/16_building-ai-agent-workflows-with-branching-and-approval.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/17_claudearchitectcertification-com-concepts-plan-mode-md.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/18_authorization-and-governance-for-ai-agents-runtime.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/19_github-gemini-cli-extensions-conductor-a-plugin-for-ai.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/20_human-in-the-loop-ai-agents-when-approval-gates-matter.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/21_gemini-cli-plan-mode-separates-thinking-from-doing-and.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/22_introducing-context-driven-development-for-gemini-cli.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/23_spec-driven-development-with-gemini-cli-by-giovanni-galloro.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/24_feature-introduce-an-ask-plan-mode-and-allow-easy-switch-an.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/25_the-openhands-software-agent-sdk-a-composable-and.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/26_openhands-deep-dive-build-your-own-guide-dev-community.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/27_github-openhands-openhands-openhands-ai-driven-development.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/28_prd-planning-agent-issue-8964-openhands-openhands.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/findings/29_chat-modes-aider.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/references/01_neel-nanda-google-scholar.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/references/02_hoang-h-tran-google-scholar.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/references/03_junyu-liu-google-scholar.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/references/04_google.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/source_plan.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/planning-mode/research/synthesis.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/steering-protocol-substrate/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/steering-protocol-substrate/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/CAVEATS.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/04_github-withzombies-hyperpowers-claude-code-superpowers-with.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/05_github-skainguyen1412-antigravity-superpowers-bring.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/06_todowrite-vs-task-in-claude-code-which-to-use-when.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/07_task-management-florianbruniaux-claude-code-ultimate-guide.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/08_tasks-api-vs-todowrite-florianbruniaux-claude-code-ultimate.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/09_feature-request-global-database-for-usage-tracking-cost.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/10_issues-aider-ai-aider-github.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/11_claude-code-ultimate-guide-guide-workflows-task-management.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/12_github-coctostan-pi-superpowers-github.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/13_github-hridaya423-conductor-tasks-a-task-management-system.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/14_github-gemini-cli-extensions-conductor-a-plugin-for-ai.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/15_github-shepalderson-copilot-orchestra-agents-and-workflow.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/17_github-aider-ai-aider-aider-is-ai-pair-programming-in-your.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/findings/18_github-pcvelz-superpowers-an-agentic-skills-framework.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/source_plan.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/task-model/research/synthesis.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/developer-settings-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/developer-settings-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/increment-2-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/increment-2-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/polish-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/polish-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/turn-end-plain-english-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/turn-end-plain-english-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/watchdog-settings-completeness-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/agent-capabilities/watchdog/watchdog-settings-completeness-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/agent-skills/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/auto-server-launch/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/autonomy-host-continuation.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/chatgpt-subscription-auth/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/cli/buffer-scrollbar/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/buffer-scrollbar/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/chat-view/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/chat-view/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/chat-view/step2-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/chat-view/step3-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/cli/chat-view/step3-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/chat-view/step4-design.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/cli/chat-view/step4-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/dev-mode/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/dev-mode/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/history-view/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/history-view/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/image-input/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/image-input/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/living-recap/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/living-recap/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/llm-provider-wireup/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/llm-provider-wireup/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/mcp-host/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/mcp-host/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/model-catalog-online/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/model-catalog-picker.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/cli/native-tool-calling/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/native-tool-calling/followups.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/cli/native-tool-calling/tasks.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/cli/session-model.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/cli/settings/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/settings/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/streaming-markdown/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/streaming-markdown/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/textarea/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/theme-system/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/theme-system/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/tui-charm-v2/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cli/tui-charm-v2/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cloud-integration/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/cloud-profile-routing.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/configurable-local-model/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/deep-research/enhancement-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/deep-research/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/deepinfra-infer.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/features/distribution/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/docker/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/document-tool/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/embedded_inference/README.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/features/embedded_inference/mistralrs-integration-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/embedded_inference/mistralrs-integration.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/embedded_inference/mistralrs-switch-download-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/embedded_inference/model-ram-estimate-calibration-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/embedded_inference/runtime-evaluation.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/engine/agnosticism.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/engine/bootstrap.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/generalize-agent/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/ide-enhancements/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/local-ai-mvp/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/local-coprocessor-tools/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/local-model-taxonomy/capability-tier-audit.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/local-model-taxonomy/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/mcp-server/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/plugin-packaging/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/project-context/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/remote-inference/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/replace-meridian/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/runtime-restart-tool.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/savings-estimation/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/semantic-search/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/setup-reset.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/setup-wizard/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/features/setup-wizard/openai-default-plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/test-fixtures/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/token-streaming/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/update-check/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/usage-telemetry/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/validator-dispatch/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/web-research/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/features/worker-open-inference-proxy/design.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/installation-macos.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/internal/ide-fixes.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/internal/refactor-cleanup.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/linux-artifact.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/local-macos-signing.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/macos-release-build.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/reference/agent-skills-guide.md` | guide/instructions/website (candidate maintained) | targeted semantic corrections; follow-up evidence above |
| `docs/reference/code-styleguides/general.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/reference/code-styleguides/go.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/reference/product.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/reference/roadmap.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/reference/tech-stack.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `docs/reference/workflow.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `docs/research/cloud-subscription-auth/README.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/cloud-subscription-auth/findings/01_building-a-browser-to-codex-bridge-via-codex-mcp-server-tou.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/cloud-subscription-auth/findings/02_hard-usage-limits-with-no-visibility-are-breaking-agent.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/cloud-subscription-auth/findings/03_a-complete-list-of-services-that-form-a-part-of-google-cloud.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/cloud-subscription-auth/findings/04_provide-custom-authentication-to-azure-openai-in-foundry.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/cloud-subscription-auth/findings/05_implement-oauth-2-0-in-windows-apps-windows-apps-microsoft.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/cloud-subscription-auth/findings/06_a-complete-list-of-services-that-form-a-part-of-google-cloud.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/cloud-subscription-auth/source_plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/cloud-subscription-auth/synthesis.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/cloud-subscription-auth/verified-findings.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/compaction/external-survey.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/research/competitive-audit.md` | historical/design/research (provisional) | inventory/link scan only |
| `docs/windows-artifact.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `efforts/activity-indicator-bounce/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/activity-indicator-bounce/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/agent-failure-diagnostics/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/agent-failure-diagnostics/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/audit-cli-color-system/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/audit-cli-color-system/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/automatic-model-context-sizing/investigation.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/automatic-model-context-sizing/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/automatic-model-context-sizing/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/autonomous-control-and-progress-guards/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/autonomous-control-and-progress-guards/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/autonomous-protocol/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/autonomous-protocol/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cached-context-usage/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cached-context-usage/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cap-tool-result-payloads/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cap-tool-result-payloads/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cercano-ai-org-migration/cutover-checklist.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cercano-ai-org-migration/execution-notes.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cercano-ai-org-migration/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cercano-ai-org-migration/results.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cercano-ai-org-migration/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cloud-auth-failover-audit/baseline.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cloud-auth-failover-audit/classification-evidence.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cloud-auth-failover-audit/coordination-evidence.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cloud-auth-failover-audit/login-evidence.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cloud-auth-failover-audit/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cloud-auth-failover-audit/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/cloud-auth-failover-audit/verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/combine-autonomous-execution-gate/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/combine-autonomous-execution-gate/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/conversation-search/performance.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/conversation-search/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/conversation-search/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/conversation-search/verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/debug-task-pane-controls/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/debug-task-pane-controls/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/deepinfra-cloud-profile-ux/merge-verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/deepinfra-cloud-profile-ux/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/deepinfra-cloud-profile-ux/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/deepinfra-cloud-profile-ux/verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/deepinfra-model-context-and-vision/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/deepinfra-model-context-and-vision/reproduction.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/deepinfra-model-context-and-vision/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/dispatch-subagent-open-model-tests/follow-ups.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/dispatch-subagent-open-model-tests/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/dispatch-subagent-open-model-tests/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/dispatch-task-taxonomy/baseline.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/dispatch-task-taxonomy/caller-inventory.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/dispatch-task-taxonomy/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/dispatch-task-taxonomy/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/dispatch-task-taxonomy/verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/fix-fallback-overflow-classification-and-compaction-budget/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/fix-fallback-overflow-classification-and-compaction-budget/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/fix-local-fallback-context-budget/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/fix-local-fallback-context-budget/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/fix-push-prompt-ux/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/fix-push-prompt-ux/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/fix-task-panel-scrollbars/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/fix-task-panel-scrollbars/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/full-request-context-meter/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/full-request-context-meter/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/glm-reasoning-content-recovery/phase3-verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/glm-reasoning-content-recovery/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/glm-reasoning-content-recovery/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/image-history-context-guard/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/image-history-context-guard/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/independent-subagent-lifecycle/acceptance.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/independent-subagent-lifecycle/execution-notes.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/independent-subagent-lifecycle/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/independent-subagent-lifecycle/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/integrate-virtualized-chat-viewport/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/integrate-virtualized-chat-viewport/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/llama-server-memory-guard/evidence-phase5.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/llama-server-memory-guard/evidence-vmstat.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/llama-server-memory-guard/followups.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/llama-server-memory-guard/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/llama-server-memory-guard/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/local-model-vision/HANDOFF.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/local-model-vision/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/local-model-vision/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/build-inputs.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/compatibility-audit.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/deployment-target-probe-followup.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/deployment-target-probe.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/discovery-test-extract.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/entrypoint-extract.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/handshake-search.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/homebrew-restart-verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/installed-startup-verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/local-notarization-verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/local-signing-verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/lookup-extract.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/offline-version-verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/release-audit.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/signed-rehearsal.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/signing-audit.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/startup-audit.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/unsigned-rehearsal.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/macos-standalone-release/update-audit.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/mcp-config-tab/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/mcp-config-tab/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/mistral-provider-integration/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/mistral-provider-integration/progress.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/mistral-provider-integration/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/new-user-reset/inventory.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/new-user-reset/offline-design-history.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/new-user-reset/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/new-user-reset/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/new-user-reset/verification.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/planning-mode-enforcement/followups.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/planning-mode-enforcement/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/primary-secondary-local-tiers/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/recommended-setup-presets/evidence.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/recommended-setup-presets/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/recommended-setup-presets/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/refactor-tool-result-finalization/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/route-aware-context-fallback/follow-up.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/route-aware-context-fallback/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/route-aware-context-fallback/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/route-aware-research-synthesis/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/route-aware-research-synthesis/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/runtime-aware-model-tiers/HANDOFF.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/runtime-aware-model-tiers/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/runtime-aware-model-tiers/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/shared-model-catalog-picker/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/shared-model-catalog-picker/progress.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/shared-model-catalog-picker/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/size-limit-diagnostics/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/size-limit-diagnostics/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/streamed-conversation-resume/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/streamed-conversation-resume/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/token-accounting/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/token-accounting/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/tune-llama-context-ram-tiers/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/tune-llama-context-ram-tiers/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/unified-model-catalog-deepinfra/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/unified-model-catalog-deepinfra/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/viewport-first-conversation-resume/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/viewport-first-conversation-resume/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/vision-as-tool/PHASE1.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/vision-as-tool/plan.md` | historical/design/research (provisional) | inventory/link scan only |
| `efforts/vision-as-tool/spec.md` | historical/design/research (provisional) | inventory/link scan only |
| `output.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-classify/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-config/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-deep-research/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-document/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-explain/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-extract/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-fetch/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-init/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-local/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-models/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-research/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-stats/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-submit-usage/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `plugins/skills/cercano-summarize/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `release/homebrew/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/enterpriseapi/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/README.md` | guide/instructions/website (candidate maintained) | corrected; targeted evidence above |
| `source/server/internal/skills/catalog/cercano-classify/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-config/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-deep-research/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-document/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-explain/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-extract/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-fetch/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-init/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-local/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-models/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-research/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-stats/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-submit-usage/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/internal/skills/catalog/cercano-summarize/SKILL.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `source/server/router_guidelines.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `test/e2e/README.md` | guide/instructions/website (candidate maintained) | inventory/link scan only |
| `website/README.md` | guide/instructions/website (candidate maintained) | inventory; website build/render tested |
| `website/app/layout.tsx` | guide/instructions/website (candidate maintained) | inventory; website build/render tested |
| `website/app/page.tsx` | guide/instructions/website (candidate maintained) | copy corrected; build/render tested |

## Final integration review (2026-10-22)

Reconciled the completed feature report with both ledgers. Independently compared
feature navigation/routing/task lifetime with CLI and agent guidance; corrected
remaining agent-index contradictions for `/cloud`, `/context` versus `/c`,
`/clear`, direct `/tool` confirmation, and `/mcp`. Confirmed independent-review
fixes remain: retired `cloud_model`, enterprise catalog discovery, temporary
protobuf comparison, and module-specific workflow/skill regeneration guidance.

Final local checks:

- Scanned all 526 tracked/untracked Markdown/MDX files: 529 relative link targets,
  including inline images and reference definitions, and 17 Markdown fragments;
  zero missing files or approximate heading-anchor misses. Fenced examples,
  external/root-relative URLs and templated targets were excluded. This is a
  syntax-based check, not browser-rendered anchor or remote URL certification.
- `GOPROXY=off GOTOOLCHAIN=local go test ./internal/skills -count=1` passed,
  including generated `.agents`/`.claude` renderer parity. All 14 canonical
  catalog skills also match their plugin copies byte-for-byte.
- `git diff --check` passed. Baseline scope is Markdown plus the two website
  copy substitutions only; no application logic, dependencies or generated
  binaries/bindings are included. Skill Markdown mirrors are intentionally included.
- Website build/render/static-export results above were not rerun: integration
  changed only Markdown, not website copy or build inputs. Existing lint and
  dependency advisories remain. No full server suite, live call, installation
  or service restart was run.

Remaining targeted-only semantic coverage after the six-guide follow-up (not
known broken documentation):
- `docs/agent/README.md`
- `docs/agent/agent-isolation/architecture.md`
- `docs/agent/compaction-decision-fidelity.md`
- `docs/agent/self-dev.md`
- `docs/cloud-routing.md`

These five paths retain bounded comparisons rather than full-file certification.
The six guides removed from this list have completed maintained-claim review,
not certification of their historical proposals. External publication and unrun
runtime acceptance remain separate from these local coverage limits.

## Six-guide publication follow-up — 2026-10-22

See [core-specialist evidence](audit-core-specialists.md) for corrections, checks
and unrun acceptance. Only documentation is changed. Publication targets
`origin/docs/publicity-refresh`, without merge, main changes or force push.
The core-guide audit remains partial; neither publication nor scoped checks
certify the entire repository. Remote SHA confirmation is reported after push,
not inferred from this pre-push report. The worktree is retained for the remaining
core-guide coverage and review; it is not deleted as part of publication.

## Core-guide publication increment — 2026-10-22

See [core-guide evidence and remaining coverage](audit-core-guides.md). The
closure-pass evidence completes maintained-claim source comparison for
`docs/agent/compaction-decision-fidelity.md`, including prompt/parser, evidence
rendering, budget/retention/merge and main-thread/dispatch wiring. It does not
certify live-model classification or repair of stored summaries.

**The requested five-guide closure remains incomplete.** These four guides
retain partial local semantic coverage:

- `docs/agent/README.md`: launcher/headless, downstream commands, permissions,
  resilience/auth, autonomy, attachment lifetime, persistence/resume and models.
- `docs/agent/self-dev.md`: dev-mode metadata, task lifecycle/routing, build and
  signing, paths, timeouts/cancellation, tracing, recap and reconnect behavior.
- `docs/cloud-routing.md`: full remaining routing/account/image/identity-storage,
  draft/save/wizard and worker-snapshot comparisons are not closed.
- `docs/agent/agent-isolation/architecture.md`: additional broker/pool/orphan and
  control-receiver comparisons corrected release/subscription semantics and
  qualified phase-era inventories; full codec/provider/credential-proxy and
  runner-core coverage remains incomplete.

Focused offline compaction and broker tests passed; details are in the evidence
report. No live external behavior, paid calls, installs, restarts or historical
proposal certification. Only documentation is included in this publication,
targeting `origin/docs/publicity-refresh` without merge or force push. Remote SHA
is verified after push, not inferred here. Retain the worktree for the explicitly
unfinished local coverage and review.

## Bounded continuation reconciliation — 2026-10-23

The latest completed slices and exact remaining checklist are in
[audit-core-remaining.md](audit-core-remaining.md). This update supersedes
overlapping earlier pending lists, not the original evidence or limitations.
Core-guide closure remains **PARTIAL**, not complete-file certification.

- README attachment-storage/lifetime bullet: source-compared, no text correction
  needed; other README families remain open as itemized in the checklist.
- Cloud routing: saved defaults/image-inheritance corrections checked; snapshot
  serialization/reconstruction now compared. Invocation/retry/refresh callers,
  identity storage, settings draft/save failures and wizard rollback remain open.
- Isolation architecture: [remaining maintained-claim source closure](audit-core-isolation-closure.md)
  completes provider construction, credential/control proxies, runner persistence
  and phase/invariant reconciliation. Earlier codec and broker/pool/control slices
  remain bounded evidence. This supersedes isolation-pending claims above, not
  historical evidence, and does not certify live isolation, parity or durability.
- Self-dev has no outstanding maintained-claim family in its saved report; no
  repeated review. Startup/headless, Permission modes, direct-tool, specialist/
  features and decision-fidelity reports retain their bounded dispositions.

Source comparison is not live acceptance. No application/dependency edits,
installs, network tests, paid calls, restarts, push, merge or land. Checks for
this continuation are recorded in the linked report, not inferred from earlier
publication passes.

## Latest remaining-list closure reconciliation — 2026-10-23

Saved reports only: [README final matrix](audit-readme-final-closure.md),
[README control closure](audit-readme-control-closure.md), and
[routing seven-family matrix](audit-routing-final-closure.md). This entry
supersedes earlier pending lists only for their recorded maintained assertions.
README autonomy, later Permission gating, subscription-auth recovery and
conversation-scoped model selection now have cross-report dispositions; routing's
seven remaining families have bounded source closure after the recorded fixes.
Earlier isolation, self-dev, startup/headless, direct-tool, specialist and
decision-fidelity dispositions remain inherited, not independently revalidated.

**Exact remaining actionable source-evidence claims:** completion decline
behavior; session-model transition/rollback details beyond validated set/clear
and next-message/resume semantics. These narrower carried-forward questions are
not explicitly closed by the control report; see the README matrix. No other
unsupported maintained claim is identified by these final saved matrices.

This closes the historical remaining list only to that extent, **not exhaustive
validation of all five complete guides or all 500 repository docs**. Runtime/UI,
OAuth/keychain/provider availability, deployed interoperability, model compliance,
exhaustive execution-path isolation, crash/disk durability and external Meridian
behavior remain unverified exclusions, not newly discovered source defects.
Source comparison and publication checks are distinct from runtime/external
acceptance. No new source investigation, tests/builds, network, guide/application
edits, push or merge in this reconciliation.

## Final two-gap closure — 2026-10-23

Supersedes the immediately preceding closure entry's two carried-forward
questions. Fresh bounded README/source comparison is recorded in the
[final README report](audit-readme-final-closure.md#final-two-gap-source-closure--2026-10-23).
Autonomous completion decline is supported: permission denial returns before
Execute, leaving that action's run/profile unchanged (`agent/toolloop.go:1110–1163`,
`capabilities/builtins/autonomous.go:213–278`, under `source/server/internal/`).
The README's session-model paragraph (`docs/agent/README.md:401–409`) does not
claim transition atomicity or rollback; that extra requirement is out of scope.
Exact set/clear validation, storage-error propagation and post-write read-back
error paths were compared (`source/server/internal/server/session_model.go:70–103,129–140`).
No concrete README mismatch found; README unchanged. **No remaining actionable
source-evidence gaps for these two maintained-claim checks.** This does not
reopen or certify other guide families, runtime/UI behavior, model compliance,
provider availability, storage atomicity or crash durability.

Publication checks cover only this ledger and the other three owned audit
reports: scoped whitespace and offline relative filesystem links (no fetching
or fragment certification). No tests/builds, runtime/network checks, application
edits, push or merge. Checkpoint restricted to the four owned audit documents.
