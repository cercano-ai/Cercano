# Closeout verification

## Baseline — 2026-11-05

Delegated repository reconciliation (task_17676b5ddbce26de95d170453cabb2b5) reported active feat/native-packages-session-safe at d971d81a39008e360ca366a86a06fadbe6f30703 after a scoped planning-doc checkpoint. Installed-worker fixes 3ebc235f3438 and 65733c5c7ec6 are ancestors. No source changes, push, or landing; active worktree retained. Main's unrelated changes were preserved. Execution plan status is maintained in this worktree.

Read-only investigation task_666bd0f0ba5cd2fb508e3a2a4585eb5e verified guest d5633dbc-c8a1-49fc-a420-df6d5d60d952 accessibility: macOS 26.5.2 / 25F84. Direct non-shell execution as test with isolated documented HOME/XDG reported Homebrew 7.0.12 and persistent HOMEBREW_NO_CLEANUP_FORMULAE=cercano-agent. Installed synthetic versions remain cercano 0.0.902, cercano-agent 0.0.901 and 0.0.902, cercano-cli 0.0.902. Reported A inode 445442 and A/B hashes, persistent configuration, synthetic markers, and policy evidence archive match prior records. This is synthetic-context protection, not system-wide policy. Initial compound probes hit Homebrew root refusal; only subsequent direct invocations establish brew results. No installs, guest lifecycle changes, models, or tests were run.

Reuse cross-platform verification lines 920–965 for package lifecycle/layout evidence and 1157–1332 for scoped cleanup retention and direct worker launchability. Those binaries predate the policy change and do not prove successful ordinary dispatch or session continuity. Reuse TestInstalledKegPoolPin, TestInstalledKegSelectionBoundaries, TestInstalledKegFormulaLayouts, TestInstalledWorkerExecutionMode, and TestInstalledAgentReuse; do not describe their deliberate handshake failure fixtures as successful turns.

A deterministic fresh-worker turn needs current-code fixtures, isolated HOME/XDG/store/socket paths, a loopback HTTP provider, observed worker identity, and bounded owned-process teardown. No real model, cloud credentials, downloads, or external service is necessary. Real local-runtime qualification remains separate.

Historical migration is excluded by explicit user direction on 2026-11-05: old-package compatibility need not be supported in this effort. This resolves applicability without asserting that no historical users existed. Omit historical monolithic-package and restart-hook migration tests; current-package continuity remains required.

No implementation, current-code native rehearsal, human acceptance, or release qualification has completed in this closeout effort.
