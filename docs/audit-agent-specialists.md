# Remaining agent specialist documentation audit

Date: 2026-10-22. Scope: the eight documents below, against this existing dirty
worktree. This is a bounded source/deterministic-test audit, not certification of
live provider behavior. Historical provider observations retain their dates.
No paid calls, installation, service restart, commits, or source edits were made.

## Coverage and corrections

| Document | Source evidence (paths relative to `source/server` unless stated) | Result |
| --- | --- | --- |
| `agent/compaction-routing.md` | `internal/loopcompact/wiring.go` (`BuildSummarizer`, `BuildConfig`, `NewFactory`, `selectSummarizer`, `summaryBudget`); `internal/compaction/execution_policy.go`; `pkg/config/routing.go` | Checked Secondary/Economy routing, ordinary resolver/fallback boundaries, six-minute caller-bounded execution budget. Added executing-model history budget (30%, 16,000 floor bounded by known window), fresh dispatch state, and local/cloud packing distinction. |
| `agent/compaction-working-memory.md` | `internal/compaction/working_memory.go`, `budgeted_summarizer.go`, `intent_scope_test.go`; `internal/compactiongen/compactiongen.go`; `internal/loopcompact/guard_scope_test.go`; `internal/agent/toolloop.go` | Removed contradictory residual SummaryGuard/rejection prose; distinguished assigned-task reference from unprompted main-chat intent hints and local budget deferral from direct cloud prompts. Preserved historical investigation evidence, not a fresh model-quality claim. |
| `agent/dispatch-diagnostic-trace.md` | `internal/dispatchhistory/history.go`; `internal/conversation/dispatch_event.go`; worker/server dispatch-event tests | Checked append-only persistence, sequence gaps, nonfatal failures, loop-level rather than wire-level snapshots. Clarified five-second sink timeout and default redaction versus response-only reasoning opt-in. |
| `agent/dispatch-reasoning-capture.md` | `internal/dispatchhistory/history.go` (`boundedReasoning`, `sanitizeBlocksWithReasoning`, `reasoningOmissionMarker`); `pkg/config/config.go`; `internal/server/server.go`; `internal/worker/worker_dispatch.go` | Corrected omission marker and clarified 32 KiB cap excludes appended notice; opaque reasoning and history snapshots remain redacted. |
| `agent/independent-tasks.md` | `internal/capabilities/builtins/dispatch_cap.go`; `pkg/config/routing.go`; `internal/taskrun/manager.go`; `internal/conversation/task_evidence.go`; `internal/server/task_subscription.go`; repository-relative `source/clients/cli/internal/ui/quiet_tasks.go` | Added class/tier/cwd/grant selection. Checked detached task lifetime, ownership, evidence window and polling cadence. Corrected stale debug-mode exception: synthetic observer-cancellation rows are hidden in main chat even in development mode. |
| `agent/mistral.md` | `internal/mistralcatalog/{store,discovery}.go`; `internal/cloudfactory`; `internal/llm/openai/mistral_content.go` and adapter tests | Clarified five-second discovery failure backoff alongside 15-minute successful cache. Kept account-scoped discovery/capability boundaries and explicitly dated live observations; no new vision, OCR, reasoning, or reliability certification. |
| `agent/proto-regen.md` | repository-relative `source/proto/generate.sh`, `source/proto/agent.proto`; `pkg/proto/agent*.pb.go`; `internal/mcp/server_test.go` | Replaced missing-procedure claim with canonical script, installation/version caveats, temporary-output comparison suitable for a dirty worktree, and test-mock/descriptor warnings. |
| `agent/vision-input.md` | `internal/toolstack/vision.go`; `internal/visioninspect/{visioninspect,locus,cache}.go`; `internal/visionattach/store.go`; `internal/capabilities/builtins/inspect_image.go`; `internal/agent/toolloop.go`; `internal/server/image_inheritance_test.go` | Historical-only dismissal was inappropriate. Added maintained inspection guidance above explicitly historical design: tool defaults, confirmed cloud capability, cloud-first leaf routing including open_primary, permitted local fallback on unavailability/failure, success-cache semantics and temporary-file attachment limits. |

### Important image limitations

The success cache is per conversation and normalized question plus image ID, not
provider/model/locus. It has no TTL or request coalescing. Tool-level availability
and lookup checks pass through; a direct cache `Inspect` hit itself does not
revalidate them. `Clear` exists, but no production cache-clear invocation was
found; tab dismissal must not be documented as cache eviction. Attachment bytes
are private temporary files, not durable session storage; IDs remain process-local.
No live image inference or export/OCR reproduction was performed.

## Validation executed

All commands below completed successfully with installed tools only.

From `source/server`, with `GOPROXY=off GOSUMDB=off`,
`CERCANO_MISTRAL_LIVE_SMOKE=0 CERCANO_MISTRAL_LIVE_CODE=0`:

```sh
go test ./internal/compaction ./internal/compactiongen ./internal/loopcompact \
  ./internal/compactor ./internal/agent ./internal/dispatchhistory \
  ./internal/conversation ./internal/taskrun ./internal/visioninspect \
  ./internal/visionattach ./internal/toolstack ./internal/mistralcatalog \
  ./internal/cloudfactory ./internal/llm/openai ./internal/capabilities/builtins \
  ./internal/hostsvc/tools ./internal/worker ./internal/server ./internal/mcp \
  ./pkg/config
```

Result: all 20 packages passed (some Go test-cache hits), exit 0, 18.86 seconds.
From `source/clients/cli`, `GOPROXY=off GOSUMDB=off go test ./internal/ui`
passed, exit 0, 4.006 seconds. These are targeted tests, not a fresh full-suite run.

The no-install temporary generation command in `agent/proto-regen.md` was
executed: both `cmp` checks matched checked-in working-tree bindings. Installed
versions: `libprotoc 34.1`, `protoc-gen-go v1.36.11`,
`protoc-gen-go-grpc 1.6.2`. The install-capable script itself was inspected, not
run. Temporary output was removed; generated files were not changed.

Scoped `git diff --check` passed. No live credentials, provider calls, deployment
or restart behavior were exercised. Earlier document claims of full-suite/live
success remain historical evidence, not results of this audit.
