# Invocation-owned startup cleanup — 2026-10-27

Bounded Phase 1 implementation, based on c66eeeb6 (with documentation-only 779d89a2). Canonical activation remains blocked; exact prepared-store selection and modelpolicy authorization are unchanged. One Agent authority per process remains required.

## Changes

Ordinary startup transfers acquired resources to an idempotent shutdown callback only after successful composition. Failure defers close the owned conversation store, cancel/join retention, close producers, listener/server, catalog, watchers, MCP, accounting, runtime and enterprise participation as applicable. Fatal exits inside composition now return errors through cleanup. Default factories and authority paths remain unchanged.

Explicit in-process and remote-error fallback managers now transfer a Close capability. Local Close fences new Start/DownloadModel work, cancels and joins admitted work/download workers, then stops remaining local instances. Remote cleanup still closes only the caller attachment; no shared StopAll is introduced.

## Isolated evidence

Injected listener/store/catalog failures check acquired resource counts, including exactly-once store/runtime/listener cleanup and catalog cancellation. Runtime tests check repeated/concurrent cleanup and sibling attachment survival. Barrier tests cover local start/download cancellation, joining and late-work refusal; retention cancellation joins before touching its nil trap store. These tests use fake dependencies, temporary paths, no native runtimes or live services.

Executed from source/server with CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off:

```
go test ./cmd/cercano ./internal/localruntime ./internal/retention -run 'Test(Production|Close|AgentTelemetry|StartCancellation)' -count=1
go test -race ./cmd/cercano ./internal/localruntime ./internal/retention -run 'Test(Production|Close|AgentTelemetry|StartCancellation)' -count=1
```

Both passed, including matching retention tests. Regression control temporarily disabled store/local-runtime closure: listener failure reported store=0; explicit inprocess and remote-error fallback tests reported missing closes. Fixes were restored in finally. An initial control attempt failed to compile (unused closer), so only the corrected assertion failures count as regression evidence. These controls are not claimed as a fresh chronological pre-implementation baseline.

## Qualification limits

Cancellation plus joining establishes the tested invocation-local work has exited. It is not retirement readiness, cross-process quiescence, native loaded-model qualification, or proof that remote sibling models survive a real host teardown. Provider cancellation cooperation is required; a noncooperative provider can delay Close. Catalog Stop still does not join an in-flight fetch; watcher cancellation is not a join. Existing producer deadlines can mark accounting incomplete; they do not establish durable successful persistence. No full successful production startup/shutdown acceptance or every downstream failure permutation is claimed. No migration, live config/DB, native credential access, installed launcher replacement, push, merge or existing-user-process changes were performed.
