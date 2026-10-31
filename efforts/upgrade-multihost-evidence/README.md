# Confirmed incompatible upgrade: multiple host targets

Base: `2e360beca6dbfe2e40ceb61220b123c7ce96ad06`, branch `fix/upgrade-discovery-wal`.
Results were produced against the working-tree correction, before checkpoint; `source_commit` in result.json is the base, not a claim that base contains the fix.

## Cause and correction

Discovery previously selected only the configured listener and required every other known writer to be its direct worker. A separately identified host therefore raised `no verified host/worker association` before the native shutdown mechanism could run. `baseline.log` retains the smallest two-host active-worker regression failure (test added before production edits).

`upgrade_darwin.go` now collects independently identified same-user agent hosts, obtains native listener evidence for each, associates workers with any verified host, and preflights the entire set before shutdown. It reuses the existing freeze, descendant classification, identity-checked TERM/KILL and exit verification for each host. Unknown roles, orphan workers, missing listeners, stale identities and unresolved children remain refusals. No idle/task-finished prerequisite was added. Discovery timing remains visible.

The existing ordering is unchanged: launcher `confirmed_upgrade` calls `upgrade-store --check-only --stop-hosts-config` before creating a backup; `runUpgradeStoreChecked` stops hosts and runs remaining-writer detection before opening any store. Compatible launches do not enter this path.

## Reproduction and verification

* Baseline: `python3 test/fixtures/production-stop/build.py test ./internal/brewrestart -run '^TestUpgradeMultipleActiveHosts$' -count=1 -v` failed with `no verified host/worker association` (baseline.log).
* Final focused run: `python3 test/fixtures/production-stop/build.py test ./internal/brewrestart ./cmd/cercano -run 'TestUpgrade|TestAutomaticUpgrade|TestOfflineUpgrade' -count=1 -v` exited 0 (targeted.log). Includes two active hosts/workers, stale identity refusal without disruption, cancellation/cleanup, descendant tests and stop failure/remaining-writer refusal before store opening. These tests use explicitly owned disposable processes or injected discovery failures, never ambient shutdown. An initial malformed wrapper invocation (`-- go test`) failed with `go go: unknown command`; corrected command above passed.
* `python3 source/server/scripts/test-ordinary-upgrade.py`: 13 tests passed, exit 0. Python emitted unclosed SQLite connection ResourceWarnings. Covers decline unchanged, repeated preflight refusal before migration, remaining/unresolved process refusal before mutation, compatible follow-up without additional shutdown/backup/migration, WAL backup and history preservation. Launcher generation/discovery/stop here are synthetic, not native acceptance.
* `python3 test/fixtures/production-stop/build.py`: exit 0; then `python3 test/fixtures/production-stop/run.py combined`: exit 0. Final disposable root `/private/tmp/cstop-wagkqkw4`; result.json and native-stop.log copied here. Two untagged production agent hosts shared the disposable store, with active workers, held provider work and a writing tool descendant. Native shutdown terminated both hosts (exit -9), checked owned descendants, and logged `verified-stop-before-backup`. One shutdown invocation, one backup, one migration to schema 2, four preserved history rows and fresh fixture reply; no remaining fixture processes after cleanup.
* `git diff --check`: passed.

## Scope and provenance

Native combined shutdown substitutes only the explicitly owned candidate PID list; identity/listener checks and signals are native. Provider is a loopback synthetic Ollama server; replacement-generation inference is a fixture. Post-stop global discovery is replaced with fixture-owned verification to avoid ambient inventory. `binary-sha256.json` records the built binary hashes. Result generation/source and worker hashes are retained in result.json. This is not live-provider, live-inventory, deployment or kernel writer-fence evidence. Survivor/remaining-writer failure coverage uses injected detector outcomes; no claim is made of manufacturing a native process immune to SIGKILL. Compatible behavior is covered by disposable synthetic launcher regression, not a live A/B session experiment.

No live configuration/database access, ambient signalling, installation, push, or inventory cleanup was performed. Existing untracked inventory evidence is not part of this checkpoint and remains retained.
