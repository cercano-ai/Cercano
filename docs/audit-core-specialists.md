# Specialist guide audit — 2026-10-22

Scope: exactly the six guides listed below, based on repository HEAD
`f2db714224467341d530f7bcdb715d8a44506dfb` plus this documentation patch.
This completes their maintained, repo-verifiable claim review, not an entire
repository audit. Historical proposal bodies are retained and superseded by
explicit current-status sections. External/live claims are not certified.

## cloud-responses.md — complete

Read the full guide and compared the client request/auth/error/capability paths,
adapter mappings, stream event handling and factory wiring. Evidence:
`source/server/internal/llm/responses/client.go:40-150,204-372`,
`adapter.go:18-64,95-104`, `stream.go:139-172`, and
`source/server/internal/cloudfactory/factory.go:60-63,97-108`.
Corrected current status for the moved provider interface, subscription route,
conditional vision, assistant output text, reasoning replay, absent ToolChoice
forwarding, stream errors, error-body diagnostics and temperature compatibility
retry. Old SDK/provider-market assertions and proposed tests remain historical;
no live provider, retention or quality validation was performed.

## locus-mode.md — complete

Read the full historical design; checked maintained task defaults against
`source/server/pkg/config/routing.go:45-57,84-102` and compatibility/defaults in
`config.go:388,637,1074-1078`. Existing supersession is accurate; added explicit
legacy migration/default and privacy boundary so old enums, default, proposed
commands and local-only guarantees cannot be mistaken for current guidance.
Historical implementation proposals were not executed or certified.

## locus-coproc.md — complete

Read the full historical scope and decisions; same routing/config evidence as
above, plus `source/server/internal/mcp/server.go:715` (research task identity).
Added the same explicit compatibility boundary: the DirectLocal/Coproc proposal,
SmartRouter assertions and four-mode table are not maintained APIs or routing
instructions. Saved destinations/quality, not a tool name, determine placement.
No paid inference or runtime routing probes performed.

## per-backend-quirks.md — complete

Read the complete maintained guide. Confirmed the backend matrix and three-field
struct in `source/server/internal/llm/openai/quirks.go:7-27`, inference and vision
gates in `internal/cloudfactory/factory.go:60-108`, transport patches and
normalization in `internal/llm/openai/transport.go:41-61,66-163,171-260`, and retry
ownership/defaults in `internal/inference/resilience/resilience.go:1-31,175-176`.
No discrepancy requiring a guide edit found. External endpoint conformance and
Mistral signed-reasoning support are not newly live-tested.

## sub_agents/README.md — complete

Read the full guide and compared grants, identity, persistence, progress and
result metadata with `source/server/internal/hostsvc/tools/tools.go:250-310,
512-585,738-895`; server wrapper: `internal/server/agentic_dispatch.go:15-22`;
registry subset: `internal/agenttools/registry.go:90-113`; class defaults:
`pkg/config/routing.go:45-57`. Added implemented-status override and corrected
the exact-list exception (managed shared-skill reader), approval scope and
unconditional child identity versus conditional persistence. UI layout remains
historical proposal, not a claim of verified UI behavior. No PTY/live child run.

## trajectory-export.md — complete

Read the full design and compared maintained server/export behavior with
`source/server/internal/server/trajectory_export.go:12-42`,
`internal/trajectory/export.go:69-179,210-350,432-510` and
`internal/trajectory/validate.go:11-68`. Added concrete current limits: ignored
log option, absent config snapshot, selected (not lossless) metadata, limited
redaction, non-recursive validation without checksum or call-reference checks,
and no guaranteed symlink isolation. Retained layout/examples/UX and acceptance
criteria as historical targets. This is not external ATIF schema or Harbor
interoperability certification and does not certify the proposed CLI/TUI flow.

## Focused offline validation

With `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local INTEGRATION_TEST=0`, selected
Go tests passed in `internal/trajectory`, `internal/llm/responses` and
`internal/llm/openai`: exporter artifact/subagent and ZIP cases, Responses
message/input/output mappings, static/token authorization, Codex endpoint
pinning, and matching quirk/transport patch cases. Selection used anchored names,
not integration tests. An initial config/tools selection matched no tests; a
second explicit run passed `TestDefaultsLocusMode`,
`TestLoad_LegacyLocalKeysBackwardCompat`, `TestRoutingSparseProfilesAndTasks`,
`TestRoutingReferences`, and `TestRunCommandLegacyDispatchGrant`.
These are focused checks, not full package or full application certification.
No installation, dependency download, restart or paid/live inference was used.

Review covered the complete documentation diff and the six per-file evidence
records. The backend-quirks guide needed no further edit. Historical proposal
bodies were preserved. The existing five-core-guide report remains **PARTIAL**.

Scoped relative-link check: nine documents (six guides plus the three audit
reports), 71 relative file targets, zero missing. Fenced examples, external URLs,
root-relative paths and templates were excluded; fragments were not validated.
Scoped `git diff --check` passed before checkpoint. No unrelated dirty files
were present at publication review. Publication is branch-only; the worktree is
retained for remaining partial core-guide coverage and review.
