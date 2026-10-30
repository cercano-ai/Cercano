# Existing-format initialization: bounded disposable probe

## Finding

An existing ordinary database is refused because generation startup requires coexistence metadata before normal persistence initialization can run—not because its conversation data requires import.

`conversation.Open` (`source/server/internal/conversation/store.go:390–457`) applies ordinary schema/migrations but does not create coexistence metadata or ownership tables. `development_launch.go:140–151` and `production_startup.go:74–91` preflight before ordinary initialization. `PrepareDevelopmentStore` exclusively creates a new file; `OpenCoexistence` deliberately does not run ordinary migrations.

The disposable probe reproduced:

```
coexistence marker required: SQL logic error: no such table: coexistence_metadata (1)
```

Preparation of the same existing private destination failed with `file exists`.

## Recommendation

Reuse normal persistence initialization with an explicit generation-compatible initialization mode. Serialize schema inspection/migration, initialize task schema without reconciling outstanding tasks, add ownership schema and declaration coherently, then use the ownership-enforcing store. Keep read-only preflight read-only. Recognized unmarked ordinary schema may be upgraded; compatible marked stores should be validated/reused; unknown or partial declarations must not be relabeled. Validate actual required structure before stamping metadata.

No separate history import subsystem is justified by this result. No history copy, identifier rewrite, or ownership backfill was needed in the tested current format; existing conversations acquire ownership lazily.

The necessary safety boundary is exclusion of old unfenced writers during initial upgrade. An existing ordinary handle can still write after additive metadata initialization, and current ordinary `Open` can reopen the marked store unfenced. Correct normal opening must reject or select ownership-enforcing behavior for marked stores before legacy migrations. SQLite transaction locking alone does not exclude an already-running old binary after migration commits. Initial upgrade therefore requires old unfenced hosts to be stopped/excluded; an available ownership lock does not authorize resetting existing owned/reserved records.

## Evidence and limitations

Investigation task `task_706db7439e281a86ae53fa8a71ca29b6` used disposable databases populated via normal `Open`, with synthetic parent/child conversations, title/recap, user/assistant turns and blocks, and a running delegated task. Its additive-schema experiment preserved history and the running task, refused an unowned mutation, and accepted an explicitly owned mutation. It also reproduced the old-handle and normal-reopen fencing bypasses.

The parent read `/tmp/cercano-db-probe.s0Wb5y/probe-final.txt`, which records these outcomes and `PASS` for `TestDisposableExistingFormatProbe`. The task additionally reports passing focused ownership/coexistence regressions. Probe source, overlay and logs are retained at `/tmp/cercano-db-probe.s0Wb5y/`.

Tests used isolated HOME, disabled credentials and offline Go dependencies. No real history was inspected or changed. This document records a recommendation and feasibility probe, not an implemented migration or production migration acceptance.
