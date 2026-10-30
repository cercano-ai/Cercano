# Bounded worker-pool race correction (2026-10-30)

This is a partial unit, NOT affirmative upgrade acceptance.

## Expected results declared before implementation

The regression assertions added before editing production code require:

- Acquire after Shutdown returns nil + error without invoking spawn.
- A spawn held across Shutdown is killed on return, returns an error, and does not repopulate the pool.

Tests use synthetic handles only; no OS worker or model process is launched, no database is opened by the test body, and no real agent is signaled.

## Reproduction

From `source/server`:

`CERCANO_DISABLE_NATIVE_CREDENTIALS=1 go test ./internal/worker -run TestPoolShutdown -count=1`

Exit 1 before the pool correction:

- Acquire after shutdown: error nil; spawns 1 (expected error, spawns 0).
- Inflight spawn after shutdown: error nil; killed false; cached 1 (expected error, killed true, cached 0).

## Regression

`gofmt -w source/server/internal/worker/pool.go source/server/internal/worker/pool_shutdown_internal_test.go`

From `source/server`:

`CERCANO_DISABLE_NATIVE_CREDENTIALS=1 go test -race ./internal/worker -run TestPool -count=1 -timeout=90s`

Exit 0: `ok cercano/source/server/internal/worker 1.929s`.

The environment variable above was set as recorded; these tests do not exercise a credential provider, and this report does not establish that this variable controls production native credentials.

## Source findings and limits

- Worker main-turn and subagent persistence use stream proxies (`internal/worker/worker.go`, `proxies.go`); `host.go` receives persistence messages and invokes host persistence callbacks. Those paths do not establish workers as independent DB writers.
- `spawn_unix.go` sets `Setpgid: true`; killing only the host group is insufficient for these workers.
- `pool.go` previously spawned outside its mutex and published the returned handle after Shutdown cleared its map. It also allowed new Acquire after Shutdown. The correction closes admission under the same mutex and kills late returning handles rather than publishing them.
- This change DOES NOT turn Shutdown into a process-exit barrier. A spawn still in flight when the host is forcibly killed remains a separate launcher identity/termination concern. Do not remove the affirmative-upgrade refusal based on these tests.
- `brewrestart/upgrade_darwin.go` currently snapshots associated candidates and waits for them after requestShutdown; verified worker termination and concurrent launch coverage are not established by this unit.

## Outstanding acceptance

No combined ordinary-launcher consent, production shutdown, verified DB-writer exit, backup, same-DB migration, normal startup, decline invariance, or compatible A/B continuity acceptance was run in this unit. No installed script was changed. The pre-existing fixture files and binaries remain untracked and untouched; only this text evidence should be staged explicitly alongside the pool correction and regression source.
