# Bounded core-guide reconciliation and README permissions — 2026-10-23

Baseline: `466cd21e42642d4ca3786b87c09d943ed5e673ae`.
Branch: `docs/core-closure-bounded`. This is documentation source comparison,
not runtime acceptance or complete-file certification.

## Unit 1 — reconcile obsolete pending ledgers

Read `audit-core-guides.md`, `publicity-freshness-audit.md`,
`audit-core-self-dev.md` and `audit-core-readme-startup.md`, then updated the
core, agent and cumulative ledgers. The self-dev completion evidence and final
readback supersede the earlier self-dev pending list; startup/headless similarly
supersedes only its bounded README families. Retained the self-dev publication
warning that not every changed claim was independently revalidated. Decision
fidelity's already-completed closure and the additional isolation/identity
comparisons were preserved, not repeated or promoted to broader certification.
Historical observations and their test results remain attributed to those passes.

### Bounded final review of saved self-dev changes

Read back only saved build/signing, log-path and tracing guidance, not a new
whole-guide audit. Compared optional identity/no-signing branches in
`source/server/scripts/codesign-if-available.sh:17–33`, auto-launch log selection
in `source/server/pkg/agentclient/client.go:175–193`, nonfatal crash capture in
`source/server/cmd/cercano/main.go:1780–1792`, and startup environment/per-fragment
sentinel/output in `source/server/internal/llm/openai/stream_trace.go:35–61`.
These saved qualifications agree with source; no self-dev edits were needed.
Other self-dev families retain the saved report's evidence and limitations,
not fresh independent revalidation. No build, signing, log creation, database
query, restart, reconnect or live smoke was performed.

## Unit 2 — README `Permission modes` only

Corrected the ordinary model-driven built-in gate's scope, removed misleading
fixed-tier examples, and documented persistence/chip and exceptional-gate
boundaries. Evidence:

- `source/server/internal/agent/permissions.go:14–90,126–147,177–182,198–202`:
  valid modes, ordinary R/W/X decisions, default permissive mode, MCP allowlist,
  session-model/human-handoff exceptions and persistence. MCP W/X policy is
  qualified because the tool loop sends effective R-tier calls down its read path.
- `source/server/internal/agent/toolloop.go:925–958,1017–1135`: argument-dependent
  tiers, profile fence, read/write split, watchdog challenges/blocks/escalation,
  per-call mode/allowlist, preauthorization and forced confirmation. Watchdog
  human escalation approval avoids a second permission prompt; bypass is not
  a universal authorization guarantee.
- `source/server/internal/agent/forced_confirm.go:28–88`: recognized shell-tool
  names and WIP-consuming stash detection. The README deliberately says
  “recognized” commands, not a complete shell/security guarantee.
- `source/clients/cli/internal/slash/permissions.go:7–36` and
  `internal/ui/model.go:3329–3345` in that CLI module: command dispatch and
  optimistic status update. `source/server/internal/server/server.go:4398–4411`
  returns failure for invalid modes, missing store or failed persistence;
  `source/server/cmd/cercano/main.go:482–483` selects the permissions file.
- Direct `/tool` behavior is cross-referenced to the existing corrected README
  text and saved core-guide evidence; its completed comparison was not repeated.

The later architecture `Permission gating` subsection and slash-command table
are not implicitly certified by this section-sized unit. Worker permission-update
transport/liveness and full permission-mode UI behavior remain outside this unit.

## Focused offline checks

With `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, from `source/server`:

```sh
go test ./internal/agent -run 'TestGateDecision|TestPermissionStore' -count=1
```

PASS: `ok cercano/source/server/internal/agent 0.498s`. This exercises gate/store
assertions, not tool-loop, worker or CLI end-to-end acceptance. A CLI attempt
using `-run TestPermission` returned `[no tests to run]`; it provides no test
coverage and is not counted as a passing permission test.

Scoped checks passed after report generation: tracked `git diff --check` and
new-report trailing-whitespace check; five edited Markdown files, 80 relative
inline filesystem targets, zero missing. The link scan excludes fenced code,
images, reference-style links, anchors and external URLs; it is not semantic
verification. No installs, restarts, inference, network-dependent tests or
application/dependency edits were performed.

## Exact remaining coverage

- **README:** prerequisites and remaining setup/model guidance; capability
  summaries outside completed headless coverage; other slash-command handlers
  and table claims; cloud/provider/local setup, authentication and resilience;
  architecture and provider/tool inventories; autonomous completion; attachment
  lifetime; persistence/resume; later architecture permission summary; known
  limitations and conversation-scoped model selection. Startup/headless, the
  earlier direct-tool comparison and this `Permission modes` section must not
  be treated as pending whole-section reaudits.
- **Cloud routing:** no new comparison in this unit. Default assignments,
  redirect/final-destination policy, account traversal/quality, image selection
  and cache, identity storage/other providers, draft/save failure isolation,
  wizard rollback and per-turn worker configuration remain per the core ledger.
  The saved advisory identity-lookup comparison does not close these families.
- **Isolation architecture:** no new comparison in this unit. Preserve the saved
  runner/protocol, pool/orphan and broker comparisons; remaining codec,
  provider/credential/control-proxy and runner-core semantics are not certified.
  Complete-file coverage remains partial; do not repeat completed subsections.
- **Self-dev:** saved maintained-claim source comparison is complete within its
  report's scope; this final review is only the spot-check above. Publication
  revalidation and runtime limitations remain. No full repeated audit is needed.
- **Decision fidelity, specialist and feature groups:** no new audit here;
  preserve their completed scoped dispositions and acceptance limitations.
