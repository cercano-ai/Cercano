# Persistence compatibility gate

Inspection 2026-11-18; no schema or user database changes performed.

Bounded audit task `task_6b9e89345e363277e4f1e82172ddeebc` reports:
- Ordinary `conversation.Open` initializes unmarked stores but refuses `coexistence_metadata` stores, rechecking inside BEGIN IMMEDIATE (`store.go:371–497`).
- `PreflightCoexistence` / `OpenCoexistence` require exact declarations and do not migrate (`coexistence.go:115–180,266–295`). Current development and fixture declarations use schema 2; their contracts differ.
- Conversation `project_dir` / `dev_work_dir` are not execution identities; tasks separately persist `work_dir`. Ownership host/epoch governs writer authority, not client executor identity.
- Existing declared stores cannot gain binding columns by merely editing startup initialization.

Directly verified `internal/conversation/offline_upgrade.go`: `UpgradeOffline` requires all legacy agents stopped. Marked databases must already match the current development declaration and user_version, then return without initialization. Unmarked stores require user_version 0 and an existing conversations table, then initialize under a transaction. There is no current declared-version-to-version migrator. `UpgradeInterruptedOffline` also reconciles tasks, never replays them, and documents a saved-backup requirement.

Before persistence implementation, select the rollout policy explicitly: support an offline versioned upgrade preserving existing host-bound conversations, or initially allow client binding only in newly prepared compatible stores while old stores retain host-only support. The former best matches eventual support for existing installations but requires migration implementation, backup/stopped-writer requirements and compatibility tests. Do not silently bump schema or repurpose existing host ownership as client authority.

Relevant tests identified by audit: TestOpenCoexistenceDoesNotInitializeOrMigrate, TestCoexistenceRefusalDoesNotMutate, TestOfflineUpgradePreservesAndRepeats, TestOfflineUpgradeRollsBack, TestOfflineUpgradeInterruptsWithoutReplay, TestOwnershipDeclarationMismatchFailsClosed. These specific conversation tests have not been run in this continuation.

## Baseline verification in this continuation

All PASS:
- source/server: `go test ./internal/context ./internal/worker ./internal/dispatch ./internal/server -count=1 -timeout=120s`
- source/clients/cli: `go test ./internal/ui -count=1 -timeout=120s`

No client execution acceptance is implied by these host baseline tests.
