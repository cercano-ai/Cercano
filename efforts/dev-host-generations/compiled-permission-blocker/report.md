# Fresh compiled gate: dispatch permission blocker

2026-10-26 checkout: `9ada58ee696fdd48a7871326dae43b39cd7be63e` (confirmed `ec7378d11c0a` is an ancestor). Working tree includes concurrent changes; compiled.json records the actual source snapshot tree, not just checkout HEAD.

Read `acceptance-unblocked.md`. Rebuilt bootstrap, published host and CLI from a new temporary source snapshot, with test-only `developmentfixture,cercano_development_acceptance` tags. No old binary reused. `compiled.json` records snapshot commit/tree, bootstrap Go build metadata, and SHA-256 of bootstrap/published host/CLI. Python uses its real interpreter rather than Apple's xcrun shim (which injects publisher-forbidden compiler overrides).

## Observed

```
CERCANO_DISABLE_CREDENTIALS=1 CERCANO_RUN_COMPILED_LAUNCHER_ACCEPTANCE=1 \
CERCANO_COMPILED_LAUNCHER_EVIDENCE=<repo>/efforts/dev-host-generations/compiled-permission-blocker \
GOPROXY=off GOSUMDB=off go test ./cmd/cercano \
-run '^TestEverydayLauncherCompiledPTY$' -count=1 -v -timeout=15m
```

FAIL after 57.82s: `timeout independent dispatch reached provider barrier`.
A was published and attached through the ordinary shell launcher, real publisher, real compiled host and real CLI PTY. Provider now receives the foreground request (the enabled-snapshot fix is present). Synthetic provider supplies explicit synthetic runtime capacity required by the normal worker graph. The harness waits for the actual approval prompt and sends `y`; CLI renders `approved — running…`, but no child provider barrier event occurs. Read-only SQL observation returned **zero delegated tasks**. No task was seeded or admission bypassed.

## Focused production issue (source-derived diagnosis)

* `source/server/internal/server/server.go:4667-4670`: `AllowToolCall` calls `refuseUnintegratedCoexistenceWork` before resolving permission. Denial has the same problem at 4680.
* `source/server/internal/server/retained_work.go:48-52`: every non-nil coexistence instance is rejected with `FailedPrecondition: coexistence ordinary work is not enabled: persistence and scheduling admission bridges incomplete`.
* `source/clients/cli/internal/ui/model.go:5077-5083`: UI prints approval before the RPC, then discards the `AllowToolCall` error.
* Worker waits for its permission response until cleanup; captured host log ends with `send perm response id=1: EOF`.

The precise RPC error above is derived from the rejection path, not captured on the wire; the observed failure is the approved-but-stalled dispatch and zero tasks. Production permission admission needs an ownership-aware bridge rather than unconditional coexistence refusal. No production source edited here.

## Scope/status

Private temporary HOME/XDG/state/runtime, memory secrets, credentials disabled, synthetic provider, offline Go proxy/sumdb, no native credential or model calls. Temporary processes are cleaned up. Evidence excludes token contents from the checkpoint.

`CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test -tags=developmentfixture,cercano_development_acceptance ./cmd/cercano -run 'TestEverydayLauncher' -count=1` passed (opt-in compiled test skipped in this separate fast gate).

The full compiled gate remains **failed/incomplete**, not accepted. B publication, independent-task survival, reconnect/zero-respawn, transfers, notification, retirement and concurrency assertions remain unverified behind the permission blocker. Existing deliberate final failure remains; no assertion lowered. Concurrent preflight / invalid-cloud configuration files untouched.
