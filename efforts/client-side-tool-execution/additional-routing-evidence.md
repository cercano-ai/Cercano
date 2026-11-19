# Additional Phase 1 routing evidence

2026-11-18. Partial inventory only; no implementation or tests. Delegated findings are identified below; they do not certify unexamined transitive dependencies.

## Mixed inference and artifacts

Bounded task `task_d1b8ef85dcd2aa762b945a114f7c1e09` inspected four capability files and helper references:

- `capabilities/builtins/local_offload.go:localCap.Execute`: host Dispatch, LocalOffload, usage recording and WantsProjectContext with WorkDir. No direct file I/O; indirect project context must use selected workspace.
- `web_research.go:researchCap.Execute`: host-installed Python/search runtime and host model caller; optional output_dir uses RunDurable for workspace resume state. Split model/search from state persistence.
- `web_deep_research.go:deepResearchCap.Execute`: same host runtime; passes both OutputDir and ProjectDir to research pipeline. Its default-output permission issue is directly confirmed below.
- `deepinfra_infer.go:deepinfraInferCap.Execute`: host configuration/profile/secrets and one authenticated HTTP POST; path attachments read files at selected location, attachment IDs instead use conversation-scoped host attachment service. Keep those identities distinct. Reads capped at 8 MiB per attachment, encoded request capped at 32 MiB. Mandatory raw-response artifacts default to `.cercano/deepinfra` under workdir; exclusive random output files, no automatic cleanup. Route file I/O separately from credentials/request execution. Do not treat raw saved artifacts as sanitized context output.

Remaining helper evidence: research durable-resume storage, runtime path helpers, attachment-store ownership/lifetime, dispatch model caller/context behavior. These are still open, not presumed safe.

## Directly confirmed permission compatibility question

`source/server/internal/capabilities/builtins/web_deep_research.go:deepResearchCap.Execute` asks for permission only when `a.OutputDir != "" && call.RequestPermission != nil`; the denial message says rerunning without output_dir produces an in-conversation report. It passes `ProjectDir: call.WorkDir` even with an empty OutputDir.

`source/server/internal/research/pipeline.go:Pipeline.Run` instead derives `<ProjectDir>/scratch/research/<topic-slug>` when OutputDir is empty, falling back to process cwd if ProjectDir is empty. It creates a progress tracker and sidecar, checks existing state and may resume it. `runPlan` calls `sidecar.Save(state)` after successful planning. `sidecar.go:Sidecar.Save` uses MkdirAll and WriteFile. Pipeline search also locates Save calls in other phases and WriteReport in synthesis. Therefore omitted output_dir does not imply read-only behavior. This is source-level confirmation, not an executed reproduction; inventory remained read-only.

Pending review: preserve current host behavior while enforcing approval for all derived output writes in client mode, or authorize correction of this permission mismatch in host mode as well. The latter changes existing approval behavior and must not be smuggled into behavior-preserving extraction. No fix has been applied.

## Automatic prompt reads

Bounded task `task_97e8c0e210b49b34a562d03681b97744` reports (paths relative to source/server/internal):

| File/symbol | Location/authority and dependencies | Routing seam / tests located |
|---|---|---|
| skills/consumer/consumer.go:Discover | Caller absolute workDir roots `.agents/skills`, `.claude/skills`, then host home roots; direct Lstat/Open/enumeration; no operation approval in discovery | Split selected-workspace discovery from host-home catalogs; consumer_test.go and boundary_test.go |
| context/context.go:Loader.Load, IsInitialized | Caller projectDir; direct context.md read/stat; cache keyed only by directory, including absence | Workspace-qualified read/cache; context_test.go |
| runner/core.go:BuildSystemPrompt, runnerDirectorySnapshot, runnerGitInfo | Automatic workDir grounding; direct ReadDir plus git branch subprocess; host env/config dependency; context through persistence seam | Route snapshot/git/context together; runner/core_test.go |
| server/server.go:buildSystemPrompt, gitInfo | Duplicate automatic directory/git grounding path | Routing runner alone is insufficient; server/toolloop_system_test.go |
| context/scanner.go:DiscoverFiles via mcp/server.go:handleInit | Explicit init, caller ProjectDir; scan then host model then context write | Split scan/write from inference; scanner_test.go |

Consumer skill metadata grants no permissions. Discovery still reads bounded body bytes before extracting metadata. Embedded skill catalog generation is separate from filesystem discovery. Context caller references include agent/agent.go, dispatch/engine.go, mcp/server.go, hostsvc/persistence/persistence.go, worker/host.go and worker proxy/context helpers. Worker transport semantics remain partially traced. No separate AGENTS.md automatic loader was established. Tests were located, not run.

## Mixed workflow operations

Bounded task `task_3704118bd4bad5e24a1ebf8b26f0319d` reports:

- `request_plan_approval.go`: workspace document validation and declared-only Git commit, versus host profile transition; commit failure retains planning. Tests: request_plan_approval_test.go and gitflow/planning_test.go.
- `plan_set_status.go`: selected-workspace taskmodel plan mutation versus host hydration/progress callbacks; underlying store I/O remains untraced. Tests: plan_set_status_test.go.
- `plan_experiment.go`: exact one-call approval consumed by host, then bounded command delegated to RunCommand; selected-workspace process execution. Tests may have arbitrary effects. Tests: plan_experiment_test.go.
- `gitflow_checkpoint.go`: repository/config resolution and CheckpointWithOptions all depend on selected workspace; no host service in wrapper. Tests: gitflow/checkpoint_test.go, gitflow_caps_test.go.
- `capture_decision.go`: host autonomy-ledger mutation, no direct workspace access in inspected implementation; backend untraced. Tests: capture_decision_test.go, autonomous_test.go.
- `gitflow_land.go`: workspace reconciliation/test/finalization may retarget another linked worktree; preserve host approval/review, no implicit push or cleanup. Lower subprocess dependencies remain untraced. Tests: gitflow/land_test.go, worktrees_test.go, builtins/gitflow_land_test.go.

Autonomous effort-artifact writers, ledger backend and underlying command/Git execution remain gaps. All tests above were located only.
