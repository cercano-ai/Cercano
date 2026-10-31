# Final checkpoint verification — 2026-10-30

## Source and scope

Initial branch `fix/upgrade-discovery-wal`, HEAD exactly `53780c6b3940d78b8c7d62ba052b1c2dc8a7b202`; `git diff --stat` empty. Read root AGENTS.md. Existing untracked inventory evidence was preserved. No production code was edited. The test overlay changes two acceptance-tag-only command files and the Python fixture; a new opt-in `_test.go` provides supplemental read-only target reporting. `test-overlay.patch` records the journey overlay. The production untagged hosts/worker and standalone preflight were rebuilt from final production source; tagged helper uses that source plus this explicit test overlay. Build metadata reports dirty for the overlay builds, not a different production revision.

## Expected before execution

`expectations.json` was written before the combined run: two native host targets; one unrelated recognized worker; one confirmation, shutdown, backup and migration; two upgrade-store invocations and real known-agent checks; one selected generation; original history unchanged plus fresh user/assistant persistence; zero remaining owned processes. Required order: confirmation -> native termination and verified exits -> real post-stop classification -> backup -> real migration -> generation startup/history/fresh persistence. History count is determined after quiescence, not guessed for racing cancelled turns.

## Commands and results

1. `python3 test/fixtures/production-stop/build.py build -o "$PWD/efforts/final-53780-verification/preflight" ./cmd/upgrade-discovery-preflight` — passed before overlay.
2. `efforts/final-53780-verification/preflight --address 127.0.0.1:50052` — exit **1**, full aggregated errors saved in `preflight.log`. No configuration loading.
3. `python3 test/fixtures/production-stop/build.py` — passed; rebuilt untagged production binary, both tagged helpers, client and native stop fixture. Build driver uses credential-disabled disposable HOME/XDG directories, offline Go module cache. Build root and output in `build.log`; binary SHA256 and Go build information retained.
4. `python3 test/fixtures/production-stop/run.py combined` — exit **0**. This is the ordinary launcher journey, not a substitute unit test. `combined.log`, `result.json`, `native-stop.log`, `upgrade-calls.jsonl`, `generation.pty` retained.
5. Supplemental read-only report: `/opt/homebrew/bin/go test ./internal/brewrestart -run '^TestFinalReadOnlyInventoryReport$' -count=1 -v`, cwd `source/server`, using the build root's `env.json` allowlist plus `FINAL_READ_ONLY_INVENTORY=1` — passed. No stop/signal code invoked; `inventory-report.log` retained. Each candidate is classified with final production `discoverUpgradeTargets`; standalone preflight above remains authoritative for aggregated whole-inventory refusal.
6. Post-run assertions checked exactly two production known-agent calls, both host exits, sibling survival/identity/socket, exclusion from descendants, history preservation and empty cleanup. `assertions.json` retained. `git diff --check` passed.

## Combined journey: PASS

Two untagged production agents shared only the disposable store, with active workers and a writing tool descendant. Both host exits were **-9** after native stop. Before consent no stop record or backup existed. The helper verified owned host/descendant exits before returning; the launcher then made one schema-0 backup and one real migration to schema 2. SQLite integrity passed. Four original history rows were preserved exactly; resume displayed BEFORE_STOP_REPLY; the selected supported generation persisted a fresh user turn and `FIXTURE_OK` assistant reply. The retained delegated task became interrupted without replay. Tool writes stopped.

The untagged sibling worker PID **34437** had the Python harness as parent, not either host. It was explicitly included in native initial candidate inventory, but not host or descendant shutdown sets. It remained the same identity and accepted a Unix-socket connection after generation startup. Only cleanup terminated/reaped it (exit -15). No shutdown signal authority was granted to it. This is code-path exclusion plus survival/socket evidence, not a kernel syscall signal trace.

Crucially the acceptance helper now invokes **real production `knownUpgradeAgents`**, not the old empty/no-op detector. A fixture PATH `ps` substitutes only the `-U UID -o pid=,comm=` inventory with the sibling PID and known executable name. Native identity inspection and `IsUpgradeNonWriter` are unchanged, and absolute `/bin/ps` native checks are untouched. Both check-only and migration calls ran this real filter; `native-stop.log.checks` records exactly two calls. Without the worker exclusion the supplied known-name row would cause production refusal.

## Full inventory: concrete remaining refusal

Read-only boundary confirmed in `cmd/upgrade-discovery-preflight/main.go`: explicit endpoint -> `brewrestart.PreflightUpgradeHost`. That function only validates endpoint, calls `ListCandidates`, and calls aggregate discovery. Discovery/identity/listener classification has no database/config open or termination path. No ambient StopUpgradeHost was invoked.

The standalone final-source preflight reported **all ten unresolved identification errors**, not just the first: PIDs **7439, 12347, 24401, 27420, 27885, 28448, 29024, 98106, 98571, 99375**. Each is `missing executable; candidate unclassified: unresolved plausible writer: no corroborated worker role`. These are actual blockers; no attribution/provenance investigation of recognized workers was repeated.

The supplemental read-only target report identified four safely classified shutdown **TARGETS, not blockers**: **38883** (`127.0.0.1:50052`), **63130** (`127.0.0.1:61589`), **70217** (`127.0.0.1:62427`), **70225** (`127.0.0.1:54243`). Paths are in the report. No target was signaled. Because unresolved candidates remain, the full inventory does not authorize shutdown/migration. The standalone CLI prints aggregated errors but does not print successful target classifications; the test-only diagnostic supplements that visibility without changing production.

## Cleanup, retained artifacts and limitations

Owned generation host/worker were cleaned up; both original hosts exited, sibling was reaped, all captured owned processes were absent at completion (`cleanup_remaining: {}`, generation `remaining: []`). Writer shells were allowed their bounded self-expiry rather than signaling unverified shells. No ambient signals, live migration/database/config/credential/inference access, installation, landing or push.

Retain worktree `/Users/bryancostanich/git_repos/cercano-ai/Cercano-upgrade-fixes` for test overlay/evidence and pre-existing untracked evidence. Retain disposable root `/private/tmp/cstop-wjp7m1kg` for full databases, logs, generated source snapshot and build products; no fixture process is intentionally retained. Build roots printed in build logs retain isolated environment/output provenance. No live-provider or deployment claim: original hosts used loopback synthetic Ollama, replacement inference/MCP used tagged synthetic fixtures. Candidate enumeration for shutdown and ps inventory for post-stop were explicitly fixture-scoped, while native shutdown/classification and real migration remained production. Inventory is an instantaneous read-only observation, not a writer fence.
