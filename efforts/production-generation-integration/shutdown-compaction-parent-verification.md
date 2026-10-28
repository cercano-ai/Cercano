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
