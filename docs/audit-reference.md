# Reference and distribution documentation audit

Date: 2026-10-22. Repository-source audit, not live service/release acceptance.
This report covers the bounded reference/distribution group; it does not replace
`publicity-freshness-audit.md`. No application implementation or dependencies were changed.

## Dispositions and evidence

| Group/files | Disposition and evidence |
| --- | --- |
| `README.md` | Retained initial corrections: separate host/client, Apple Silicon macOS target, experimental unsigned Linux/Windows, protocols versus shared skills versus exported skill instructions. `scripts/build-macos-release.sh:17,61-80` enforces host/architecture/deployment target and two binaries. Release URLs and tap availability are not live-validated. |
| `AGENTS.md`, `CLAUDE.md` | Corrected skill authoring path to embedded catalog and generated mirrors. Routing caveats retained. `source/server/internal/skills/skills.go` and its generated-tree test establish source of truth. |
| `website/README.md`, `website/app/page.tsx` | Removed unconditional free/private local marketing and asserted live Pages configuration. Pages workflow and package scripts establish build intent, not remote hosting state. Static export tested. Other website assets/layout are presentation, not independent functionality evidence. |
| `source/server/README.md` | Retained split-binary/runtime/build instructions; module Go directive and macOS builder support build requirements. Runtime installation is distinct from building the host. |
| `source/enterpriseapi/README.md` | Removed open-PR currentness claim and incorrect future-product wording for integrity/activation. Wire contract is not host enforcement. `internal/enterprise/client.go:330-395,449-471` verifies signatures, rechecks lease after downloads and atomically stores bundle. |
| `docs/enterprise-connection.md`, `enterprise-managed-routing.md`, `enterprise-runtime-enforcement.md`, `enterprise-shared-skills.md` | Retained managed/blocked state, online verification, physical-attempt authorization and text-only shared-skill distinctions. Evidence: `internal/enterprise/{host,authorize,client,rpc}.go`, enterprise test suite. `keychain_other.go` rejects non-Darwin credential storage; macOS pilot limitation retained. Corrected connection-guide catalog scope: `source/server/internal/server/skills.go:27-55` combines built-in tool skills, protocols and managed assignments, not personal/repository directory discovery. |
| `docs/enterprise-implementation-status.md` | Historical integration/acceptance record; existing disclaimer retained. PR, pilot and deployment claims are not fresh remote-state evidence. |
| `docs/installation-macos.md`, `linux-artifact.md`, `windows-artifact.md`, `local-macos-signing.md`, `macos-release-build.md` | Retained platform/signing boundaries and packaging instructions. Source evidence: release builders, verification scripts and workflow structural tests. Checksums are not publisher authentication; cross-builds do not establish native support. |
| `release/homebrew/README.md` | Retained template/render/promotion safeguards and explicit external signing/tap limits. No formula published, tap queried or installed service restarted. |
| `test/e2e/README.md` | Retained isolated fake-provider test instructions. `test/e2e/helpers/sandbox.mjs` creates isolated home/config/temp/port and overrides cloud credentials. PTY suite itself was not run in this audit. |
| `docs/README.md` | Reference navigation now distinguishes historical product/roadmap/stack from maintained guides and links this report. |
| `docs/reference/product.md`, `roadmap.md`, `source/server/router_guidelines.md`, `docs/features/agent-capabilities/README.md` | Added historical/concept/planning warning at entry points; preserved original bodies. Binary LocalModel/CloudModel classifier is not current routing guidance. |
| `docs/reference/tech-stack.md` | Existing historical warning retained. Not an installation checklist. |
| `docs/reference/workflow.md` | Corrected skill authoring/regeneration workflow, canonical protobuf script, module-specific setup/build/test/coverage/vet commands, and absent root MCP configuration claim. Evidence: `source/proto/generate.sh`, the three `source/**/go.mod` files, server/CLI Makefiles, and `source/server/cmd/cercano/main.go`. Process conventions and coverage targets are not claims that every module meets them. |
| `docs/reference/agent-skills-guide.md`, `docs/reference/code-styleguides/*` | Retained prompt-instruction versus executable-tool distinction and style guidance. Host discovery remains conditional, not universal integration support. |
| `docs/co-processor/README.md`, `legacy-readme.md` | Existing deprecated/historical boundary retained; no runtime removal inferred. |
| `efforts/**`, `docs/archive/**`, `docs/bugs/**`, `docs/decisions/**`, `docs/research/**`, `docs/internal/**`, feature research trees | Historical design, bug, migration or research evidence preserved, not rewritten as current guides. Source citations and old status checklists do not establish present functionality. |

## Skill distribution corrections

Canonical files: `source/server/internal/skills/catalog/*/SKILL.md`; corresponding
copies in `.agents/skills/`, `.claude/skills/` and `plugins/skills/` are aligned.
Claude copies retain the renderer's result-display section.

- `cercano-local`, `classify`, `explain`, `extract`, `summarize`, `document`, `init`,
  `research`, `deep-research`: qualify model destination, privacy and cost;
  remove Ollama-only prerequisites and unconditional local synthesis claims.
- `cercano-models`: active local runtime, not necessarily Ollama.
- `cercano-config`: document `mistralrs` and rejected `cloud_model`; remove example
  that would fail. Evidence: `internal/server/server.go:1581-1583,1623-1626,2591`
  (`UpdateConfig`, `ListModels`). MCP schema/empty-result wording still contains
  legacy Ollama/cloud-provider terminology; implementation was deliberately not changed.
- `cercano-submit-usage`: automatic submission depends on host hook configuration.
- `cercano-fetch`, `cercano-stats`: retained tool contracts; no live web/telemetry
  acceptance inferred from copy parity.
- Routing evidence: `internal/capabilities/builtins/coproc.go:24` selects the
  Reconnaissance task; MCP forwards requests to the server. A legacy tool name
  is not a destination constraint. Model confidence is not calibrated verification.

## Remaining features inventory

The exact file dispositions below separate maintained functional references from
specifications/plans and research. “Retained reference” means source-scoped guidance,
not live-provider acceptance or a new claim that every edge case was exercised.
The maintained standalone references use `internal/server/setup_reset.go`,
`autonomy_continuation.go`, and `internal/capabilities/builtins/{restart_runtime,deepinfra_infer}.go`.
Restart documentation correctly scopes restart to managed llama-server, not the host.
Cloud-profile routing is a navigation stub to the separately owned routing guide.
Setup-wizard copy is retained as UI guidance; its accompanying plan is historical.

| File | Disposition |
| --- | --- |
| `docs/features/adk-integration/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/README.md` | Historical entry point; explicit warning retained/added |
| `docs/features/agent-capabilities/capability-architecture/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/capability-architecture/plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/dispatch-engine/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/dispatch-engine/implementation-notes.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/dispatch-engine/plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/git-workflows/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/git-workflows/plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/planning-mode/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/planning-mode/research/README.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/01_agent-to-human-handoff-patterns-designing-escalation-that.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/02_getting-started-with-conductor-for-gemini-cli-kdnuggets.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/03_32-claude-code-tricks-that-actually-change-how-you.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/04_plan-mode-in-claude-code-think-before-you-codewithmukesh.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/05_how-to-use-claude-code-as-a-product-manager-2026.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/06_what-is-plan-mode-in-claude-code-claudelog.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/07_planner-executor-evaluator-loop-architecture.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/08_how-to-use-aider-in-2026-setup-architect-mode-git-wor.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/09_coding-tool-reviews-aider-s-architect-mode-and-benchmarks.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/10_aider-s-architect-mode-when-the-slower-more-expensive.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/11_step-by-step-guide-refactoring-a-large-rust-codebase-with.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/12_aider-s-architect-mode-plan-before-you-code-ai-stack-today.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/13_exec-plan-ai-first-ssot.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/14_human-in-the-loop-ai-agents-when-approval-gates-matter.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/15_claude-code-plan-mode-design-review-first-datacamp.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/16_building-ai-agent-workflows-with-branching-and-approval.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/17_claudearchitectcertification-com-concepts-plan-mode-md.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/18_authorization-and-governance-for-ai-agents-runtime.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/19_github-gemini-cli-extensions-conductor-a-plugin-for-ai.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/20_human-in-the-loop-ai-agents-when-approval-gates-matter.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/21_gemini-cli-plan-mode-separates-thinking-from-doing-and.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/22_introducing-context-driven-development-for-gemini-cli.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/23_spec-driven-development-with-gemini-cli-by-giovanni-galloro.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/24_feature-introduce-an-ask-plan-mode-and-allow-easy-switch-an.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/25_the-openhands-software-agent-sdk-a-composable-and.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/26_openhands-deep-dive-build-your-own-guide-dev-community.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/27_github-openhands-openhands-openhands-ai-driven-development.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/28_prd-planning-agent-issue-8964-openhands-openhands.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/findings/29_chat-modes-aider.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/references/01_neel-nanda-google-scholar.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/references/02_hoang-h-tran-google-scholar.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/references/03_junyu-liu-google-scholar.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/references/04_google.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/source_plan.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/planning-mode/research/synthesis.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/steering-protocol-substrate/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/steering-protocol-substrate/plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/task-model/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/task-model/research/CAVEATS.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/README.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/04_github-withzombies-hyperpowers-claude-code-superpowers-with.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/05_github-skainguyen1412-antigravity-superpowers-bring.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/06_todowrite-vs-task-in-claude-code-which-to-use-when.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/07_task-management-florianbruniaux-claude-code-ultimate-guide.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/08_tasks-api-vs-todowrite-florianbruniaux-claude-code-ultimate.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/09_feature-request-global-database-for-usage-tracking-cost.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/10_issues-aider-ai-aider-github.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/11_claude-code-ultimate-guide-guide-workflows-task-management.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/12_github-coctostan-pi-superpowers-github.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/13_github-hridaya423-conductor-tasks-a-task-management-system.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/14_github-gemini-cli-extensions-conductor-a-plugin-for-ai.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/15_github-shepalderson-copilot-orchestra-agents-and-workflow.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/17_github-aider-ai-aider-aider-is-ai-pair-programming-in-your.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/findings/18_github-pcvelz-superpowers-an-agentic-skills-framework.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/source_plan.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/task-model/research/synthesis.md` | Historical research; preserved, external findings not revalidated |
| `docs/features/agent-capabilities/watchdog/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/developer-settings-design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/developer-settings-plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/increment-2-design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/increment-2-plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/polish-design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/polish-plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/turn-end-plain-english-design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/turn-end-plain-english-plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/watchdog-settings-completeness-design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-capabilities/watchdog/watchdog-settings-completeness-plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/agent-skills/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/auto-server-launch/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/autonomy-host-continuation.md` | Retained reference; implementation entry points above |
| `docs/features/chatgpt-subscription-auth/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/cloud-integration/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/cloud-profile-routing.md` | Retained navigation stub; destination guide separately owned |
| `docs/features/configurable-local-model/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/deep-research/enhancement-plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/deep-research/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/deepinfra-infer.md` | Retained reference; implementation entry points above |
| `docs/features/distribution/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/docker/plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/document-tool/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/embedded_inference/README.md` | Historical entry point; explicit warning retained/added |
| `docs/features/embedded_inference/mistralrs-integration-plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/embedded_inference/mistralrs-integration.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/embedded_inference/mistralrs-switch-download-plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/embedded_inference/model-ram-estimate-calibration-plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/embedded_inference/runtime-evaluation.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/engine/agnosticism.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/engine/bootstrap.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/generalize-agent/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/ide-enhancements/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/local-ai-mvp/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/local-coprocessor-tools/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/local-model-taxonomy/capability-tier-audit.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/local-model-taxonomy/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/mcp-server/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/plugin-packaging/plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/project-context/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/remote-inference/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/replace-meridian/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/runtime-restart-tool.md` | Retained reference; implementation entry points above |
| `docs/features/savings-estimation/plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/semantic-search/plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/setup-reset.md` | Retained reference; implementation entry points above |
| `docs/features/setup-wizard/README.md` | Retained UI reference; no interactive acceptance run |
| `docs/features/setup-wizard/openai-default-plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/test-fixtures/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/token-streaming/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/update-check/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/usage-telemetry/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/validator-dispatch/plan.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/web-research/spec.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |
| `docs/features/worker-open-inference-proxy/design.md` | Specification/design/plan or dated implementation evidence; preserved, not current setup guidance |

## Checks executed and validation limits

- `go test ./internal/skills ./internal/enterprise` from `source/server`: passed.
- Re-ran `go test ./internal/skills` after final mirror edits: passed.
- `python3 scripts/test-release-workflow.py`: 47 passed (structural/stub checks).
- `python3 scripts/test-linux-release-build.py`: 14 passed.
- `python3 scripts/test-windows-release-build.py`: 11 passed.
- `npm run test:pages` from `website`: production static export and 2 tests passed.

The release test fixtures create disposable repositories internally; no project
commit, publication, installation or push was performed. These tests are not native
Linux/Windows execution, Apple signing/notarization, real Homebrew promotion or a
GitHub Actions deployment. Outstanding external acceptance: live release URLs and
digests, tap install/upgrade, signing identities/notarization, live Pages settings,
Google organization login/Keychain permission UX, enterprise control-plane deployment,
provider access/model availability, web search availability and real runtime hardware.

Excluded ownership: `docs/agent/**`, `docs/cloud-routing.md`, `docs/features/cli/**`,
`source/clients/cli/**`, and shared `docs/publicity-freshness-audit.md` were not edited.
This is a scoped semantic/documentation audit, not exhaustive verification of every
historical sentence, external research citation or implementation branch.
