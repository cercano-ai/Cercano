# Parent verification of approved shutdown and compaction work

2026-10-27. These are bounded implementation checks, not activation or full production acceptance. User approved explicit incomplete shutdown, deferred resource disposal, no credential-helper process/forced termination, and continuation of outstanding compaction work. The decision is recorded in shutdown-decision.md and the autonomous decision ledger.

## Parent-executed checks

All Go checks used CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off.

- Initial shutdown barrier selection: `go test -race ./cmd/cercano -run 'TestProduction(SingleHostStartupShutdown|IncompleteShutdownControl|FailureStages|ListenerFailureClosesOwnedStore)$' -count=2 -timeout=90s` PASS.
- Owned compaction and persistence: `go test -race ./internal/compactiongen ./internal/hostsvc/persistence -count=2` PASS.
- Task/shutdown join regression selection across server/taskrun/cmd, count=2: PASS.
- Remote status/admission selection across cmd/server/enterprise/recap/compactiongen/agentclient, count=2: PASS.
- CLI UI `go test -race ./internal/ui -run TestShutdown -count=3`: PASS.
- Combined selection after scheduler admission c02b585e85ac:

```
go test -race ./cmd/cercano ./internal/server ./internal/taskrun \
  ./internal/compactiongen ./internal/hostsvc/persistence \
  ./internal/ollamacatalog ./internal/retention \
  ./internal/hostsvc/providers ./internal/hostsvc/permissions ./pkg/agentclient \
  -run 'TestProduction|TestPreparedShutdown|TestPreparedResource|TestShutdown|TestTaskShutdown|TestOwned|TestRetire|TestCoexistenceLifecycle' \
  -count=2 -timeout=180s
```

PASS in all packages; hostsvc/permissions had no matching tests, not exercised by this selection.

- `go test ./... -run '^$'` PASS in server and CLI modules. Compile-only, not full test-suite execution.

The stale TestProductionCompositionServiceParity failure was reproduced by parent before its test-only repair; credential admission was not weakened or enabled to restore the invalid older evidence.

## Independent reports and limits

- Compaction worker reports 20 repetitions of panic/backlog/concurrent drain/SQLite fake-summary coverage under race detection. Review found no confirmed valid-admission defect, with scoped tests subsequently strengthened.
- Protocol regeneration task reports all four generated files byte-identical after offline pinned-generator regeneration. No dependency changes or global tool install.
- A prepared-host worker accidentally selected the subprocess-based TestDevHostCompiledExecution and timed out. This is NOT passing acceptance evidence. Source-only follow-up cannot establish whether test children remained; no live process inventory or termination was performed. Existing approved plan permits isolated test children, but this exceeded that worker's narrower in-process test instruction. No credential-helper architecture was introduced.
- Read-only review after passing combined tests found ordinary cleanup error loss/false successful completion and unqualified bounded constructor rollback. Follow-up implementation tasks are active; preceding passes do not resolve those findings.
- Canonical generation activation remains disabled. No full-suite, live credential/provider/model qualification, migration, installed launcher change, push or merge is claimed by this note.

## Final bounded-slice follow-up

Subsequent fixes retain ordinary cleanup errors (`f2614938de73`), make prepared constructor failure disposal bounded and retained (`80623e40a929`), and propagate the newly returned server shutdown error through prepared adapters (`f6d1c87d7292`). The last adapter regression reproduced false successful completion before correction. Its enabled-policy scope injects only a memory credential opener and fake runtime/audit closers; it executes no turn, provider request or native credential backend. The parent read that fixture before executing it.

After f6d1c87d7292 the parent executed the combined race selection twice across cmd/cercano, server, taskrun, compactiongen, persistence, ollamacatalog, retention, providers and agentclient. Selector:

```
TestProduction|TestPrepared|TestShutdown|TestServerShutdown|TestTaskShutdown|TestOwned|TestRetire|TestCoexistenceLifecycle|TestAgentShutdownLeavesShared|^TestDevelopmentProductionCleanupShutdownError$
```

All nine packages passed (exit 0; 21.484 seconds total). This does not select the subprocess-based TestDevHostCompiledExecution. Earlier final-state parent compile-only and build checks passed in both modules after the error-aware ordinary/prepared constructor changes; the last prepared adapter changes also compiled in the final race run. CLI shutdown/reconnect race checks passed twice. No whole-suite or activation claim follows.

The bounded compaction implementation and shutdown corrections are committed; Phase 1 composition and Phase 2 producer integration remain in progress because full service parity, other producers and acceptance are not finished. The timed-out compiled fixture remains failed evidence, not silently superseded by in-process tests.
