# Publicity documentation freshness audit

Baseline: `d93f490490f6b46693d6493868ae4d85871f2396`. Branch: `docs/publicity-refresh`.

## Scope and coverage status

**Partial claim audit, not a release sign-off.** The inventory below covers all
tracked Markdown/MDX/RST/AsciiDoc plus website TSX/HTML at the baseline (521
files). Every tracked Markdown file received the bounded local-link scan described
below; inventory inclusion does not mean every sentence was source-verified.
Untracked efforts and user work were excluded. Historical/design records remain
unchanged except contextual notices where a current landing page pointed at stale
claims. Path-based classifications are provisional; mixed directories require
individual status reading. This pass does not establish freshness of every
maintained guide. In particular, plugin skill mirrors, all feature details,
installation signing guarantees, enterprise acceptance, and every CLI help entry
still need exhaustive claim-by-claim review.

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
  inference. No pushes or landing.

## Independent review

Read-only adversarial review of the diff and source found no concrete false claim
in the targeted corrections. It confirmed that this report explicitly remains a
partial audit. Review is not certification of all 521 inventoried files.

## Unverified external claims and remaining work

- Homebrew availability/version, signed/notarized release assets, deployment
  targets on actual machines, and Linux/Windows release availability.
- Provider subscription eligibility/terms, exact model availability, latency,
  cost savings and current pricing; hosted Mistral live acceptance remains unverified.
- Enterprise external repository PR/acceptance status, deployed services and
  organization-specific security guarantees.
- Skill copies still need comparison with MCP/native adapters; no blanket
  assertion that all plugin tools execute locally or have zero cost.
- Full command/help parity and remaining feature/reference claims are not certified.
- Source tests passing do not validate claims of privacy for arbitrary network
  tools or credential exports. Local inference does not imply offline operation.

## Follow-up: skill example and preexisting local-link targets

This follow-up is **not completion of the maintained-document semantic audit**.
The remaining CLI, specialist, enterprise, plugin, release, and platform groups
still require source comparisons; they are local work, not external-only blockers.

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

`corrected` means targeted corrections above, not exhaustive verification.
`inventory/link scan only` explicitly marks outstanding semantic audit work.

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
