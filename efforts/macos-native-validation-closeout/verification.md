# Closeout verification

## Baseline — 2026-11-05

Delegated repository reconciliation (task_17676b5ddbce26de95d170453cabb2b5) reported active feat/native-packages-session-safe at d971d81a39008e360ca366a86a06fadbe6f30703 after a scoped planning-doc checkpoint. Installed-worker fixes 3ebc235f3438 and 65733c5c7ec6 are ancestors. No source changes, push, or landing; active worktree retained. Main's unrelated changes were preserved. Execution plan status is maintained in this worktree.

Read-only investigation task_666bd0f0ba5cd2fb508e3a2a4585eb5e verified guest d5633dbc-c8a1-49fc-a420-df6d5d60d952 accessibility: macOS 26.5.2 / 25F84. Direct non-shell execution as test with isolated documented HOME/XDG reported Homebrew 7.0.12 and persistent HOMEBREW_NO_CLEANUP_FORMULAE=cercano-agent. Installed synthetic versions remain cercano 0.0.902, cercano-agent 0.0.901 and 0.0.902, cercano-cli 0.0.902. Reported A inode 445442 and A/B hashes, persistent configuration, synthetic markers, and policy evidence archive match prior records. This is synthetic-context protection, not system-wide policy. Initial compound probes hit Homebrew root refusal; only subsequent direct invocations establish brew results. No installs, guest lifecycle changes, models, or tests were run.

Reuse cross-platform verification lines 920–965 for package lifecycle/layout evidence and 1157–1332 for scoped cleanup retention and direct worker launchability. Those binaries predate the policy change and do not prove successful ordinary dispatch or session continuity. Reuse TestInstalledKegPoolPin, TestInstalledKegSelectionBoundaries, TestInstalledKegFormulaLayouts, TestInstalledWorkerExecutionMode, and TestInstalledAgentReuse; do not describe their deliberate handshake failure fixtures as successful turns.

A deterministic fresh-worker turn needs current-code fixtures, isolated HOME/XDG/store/socket paths, a loopback HTTP provider, observed worker identity, and bounded owned-process teardown. No real model, cloud credentials, downloads, or external service is necessary. Real local-runtime qualification remains separate.

Historical migration is excluded by explicit user direction on 2026-11-05: old-package compatibility need not be supported in this effort. This resolves applicability without asserting that no historical users existed. Omit historical monolithic-package and restart-hook migration tests; current-package continuity remains required.

At baseline, no implementation, current-code native rehearsal, human acceptance, or release qualification had completed in this closeout effort.

## Phase 2 first deterministic ordinary turn — recovery, 2026-11-05

Recovered the existing `internal/server/installed_worker_turn_test.go` left by task cb7d17e5fba6a412a36e9e4ec02f92db; preserved its test and formatted it with gofmt. Read AGENTS.md and this effort's spec/plan. No plan status changed, no production code changed, and no admission/routing/store architecture was added. The first actual test probe passed: there was no compile/runtime failure requiring a fix, and the earlier task's tool-launch failures are not evidence of an application defect.

`TestInstalledWorkerSuccessfulTurnAfterAliasSwitch` selects installed keg A, asserts it does not confer prepared authority, configures the ordinary server worker runner, verifies no worker has started, switches the public alias to B, then invokes `Server.StreamProcessRequest`. It observes one loopback provider request, exact final text, a distinct child PID reporting the canonical A executable, and persisted user/assistant turns. The child is a copied current-code test executable whose worker-only entry point serves the real worker gRPC implementation with an injected memory secret store. The parent does not launch a worker directly or call worker RunTurn; ordinary dispatch/pool/handshake/snapshot/provider execution is exercised. This is an isolated server integration fixture, not a packaged CLI/native rehearsal or proof of ordinary production startup composition, reconnect, B admission, cross-version compatibility, or local-runtime safety.

Safety: credentialtest.Enable isolates HOME and XDG directories and clears credential environment; conversation storage, socket/pid paths, keg copies and identity marker are owned temporary state. Both parent and child default HTTP transports reject non-loopback destinations; the only provider is httptest with a synthetic key and deterministic SSE response. No real inference, cloud request, live agent, VM, install, or external model/download was used. Cleanup invokes CleanupOwnedWorkers and removes temporary state. Logs retain a default model label, but all actual provider HTTP goes to the loopback fixture. The minimal conversation-store helper emits the nonfatal `no such table: delegated_tasks` completion-boundary warning; the turn and both persisted messages succeed. This does not establish delegated-task completion bookkeeping and is not a basis for a production storage change in this bounded slice.

Verification used simple argv commands with absolute cwd `/Users/bryancostanich/git_repos/cercano-ai/Cercano/wt-native-packages/source/server`:

- `go version`: exit 0, `go1.26.3 darwin/arm64` via PATH (no hardcoded Go executable path).
- Initial unchanged test probe: `go test ./internal/server -run '^TestInstalledWorkerSuccessfulTurnAfterAliasSwitch$' -count=1 -timeout=60s -v`: PASS, fresh A PID 88600, one provider request, two persisted turns.
- Final normal: `go test ./internal/server -run '^TestInstalledWorker(ExecutionMode|SuccessfulTurnAfterAliasSwitch)$' -count=1 -timeout=60s`: PASS, package 1.142s, command 4.661s.
- Final race: `go test -race ./internal/server -run '^TestInstalledWorker(ExecutionMode|SuccessfulTurnAfterAliasSwitch)$' -count=1 -timeout=90s -v`: PASS, package 2.955s, command 8.434s; fresh A PID 88654 after B alias selection, one provider request, two persisted turns; no race report. Race instrumentation also applies to the copied child test executable.

Only the first deterministic-turn case is completed here. Remaining Phase 2 work and all later phases remain as recorded in the unchanged plan. No push or native/release acceptance is claimed.
