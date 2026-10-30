# Bounded settings convergence (2026-10-29)

Baseline: f796df63. Source work, not a runtime qualification claim.

Production composition now uses ordinary config.Load (including coordinator snapshot and supported environment overrides), opts into writable settings, and starts the existing lifecycle-drained config watcher. Default prepared servers remain frozen; native credentials remain read-only. RPC mutation admission uses an exact retained terminal/owner ticket, without acquisition or transfer. Only tool-loop limits and tool-result elision are integrated; provider/runtime, credential and producer changes remain gated. Permission mode mutations use the same owner admission and existing coordinated permission store.

Publication and application are separate: RPC saves use SaveAndSet; watcher adoption uses the existing configApplyMu/revision-check/Set path, never saves. Unsupported external revisions remain durable but unapplied; their snapshots are not adopted as a future write baseline. A subsequent stale whole-file save must conflict, not overwrite those revisions.

Concrete source counterexample at baseline: production composition constructed config by YAML unmarshalling without a coordinator snapshot, applied WithSharedReadOnly, and prepared construction froze the service; updateConfigLocked unconditionally returned ErrFrozen for coexistence. Thus even a correctly bound ordinary tool-loop patch could not converge. The added TestProductionGenerationSettingsConvergence exercises that composed path, exact binding, permission publication/visibility, watcher application without rewriting, and unsupported-revision conflict retention. The existing injected-graph test now expects supported nonsecret port overrides. These tests have NOT executed here; no runtime reproduction or pass is claimed.

Verification attempted with outer CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off, cwd source/server:

```
go test ./cmd/cercano -run '^TestProductionGenerationSettingsConvergence$' -count=1 -timeout=60s
```

Direct invocation failed: `fork/exec /opt/homebrew/bin/go: no such file or directory`. Retrying through bash failed: `fork/exec /bin/bash: no such file or directory`. Earlier source/formatting commands did execute successfully. Child execution availability is inconsistent; this is not a test result. Source signature inspection caught and corrected config.Save usage and string Port types. Required narrow follow-up command (not run):

```
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test ./cmd/cercano -run '^(TestProductionGenerationSettingsConvergence|TestProductionGenerationPermissionsHost|TestDevelopmentProductionInjectedGraph)$' -count=1 -timeout=90s
```

All added test data uses disposable fixture paths and the existing invocation-local fake graph. No activation, live data, migration, network, process termination, or MCP work was performed. Phase 3 remains in progress, not marked complete. Broader parity and runtime verification are not established by this source checkpoint.
