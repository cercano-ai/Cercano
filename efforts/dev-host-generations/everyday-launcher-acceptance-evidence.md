# Ordinary launcher final acceptance — PASS

2026-10-26. Supersedes the earlier blocked/busy-reservation diagnostics.

Tested base: `528a759283db261a9cd47596e2b444da9a483965` (includes UI fix `3c7679ac`). Fresh private source snapshot commit: `7cd3f4e5392907c380f5743bfbbd6ad4a38aa35b`; source tree: `45420d3e5f31d92800ed6d1f2551578462fceb1c`. Snapshot includes the acceptance-only provider notification instrumentation and test harness changes in this checkpoint. Full compact hashes and checks: `everyday-launcher-final-528a-manifest.json`.

Executed from `source/server`:

```sh
CERCANO_DISABLE_CREDENTIALS=1 \
CERCANO_RUN_COMPILED_LAUNCHER_ACCEPTANCE=1 \
CERCANO_COMPILED_LAUNCHER_EVIDENCE=/tmp/launcher-final-528a-PASS \
go test ./cmd/cercano \
  -run '^TestEverydayLauncher(CompiledPTY|EntrypointRefusalsPTY)$' -count=1 -v
```

Both tests PASS, no skipped tests/assertions (54.126s package result).

## Compiled production journey

- Actual shell → actual Python publisher → freshly compiled tagged production host, CLI PTYs and independent worker. Synthetic provider/memory secrets only; no fake host, native credentials or model calls.
- A starts an ordinary permitted independent dispatch. Durable task is observed `running`, blocked at provider barrier; no task/notification rows are seeded.
- Kill A CLI physically and reconnect its saved selection to the exact same host PID without build or acquisition; task remains running.
- Publish source-byte-changed B while A CLI/task survive. Same-generation and two concurrent unchanged-source invocations reuse B without rebuilding.
- Saved-selection management reconnect performs no build/acquisition. Unrelated Y transfers from A to B while A task runs.
- Busy A transfer refuses, with B `pending_incoming` unchanged **0 → 0**.
- With both real A/B hosts live and task still running, fail a candidate build through the same ordinary entrypoint. The original run injected a compiler-wrapper exit before Go execution. The strengthened rerun below instead submits invalid Go source to the real compiler and requires its file-specific syntax-error diagnostic. Existing generations, host records/token bytes, selections and entire task row remain unchanged; both hosts remain manageable. Neither check is the mock publisher negative fixture.
- Release barrier: task becomes `completed`; exactly one provider completion-notice event, durable `notification_pending=0,wake_claimed=1`, no duplicate after settling.
- Retirement refuses while A retains sessions. Explicitly release actual retained parent/child sessions using observed epochs, then retire A. Child may already have been released by the real worker; never fabricate a child task.
- B performs fresh inference and returns a fresh visible response after A retirement. Release B selections (including transferred Y and concurrency sessions), retire B.

## Distinct safety fixture

`TestEverydayLauncherEntrypointRefusalsPTY` passes legacy-state immutable refusal and explicitly **mock publisher** failure preservation. This negative-entrypoint fixture is not evidence for compiled-host behavior or real failed build preservation.

## Private artifacts and boundaries

- Raw run evidence: `/tmp/launcher-final-528a-PASS` (mode 0700).
- Preserved source checkout, binaries, private state/database and raw token files: `/private/tmp/ca-suu_tvjs` (mode 0700).
- Earlier untracked repository diagnostics moved to `/tmp/launcher-final-528a-PASS/prior-untracked`; no raw PTYs, selections, logs or tokens added to this checkpoint.
- Prior harness retries remain private under `/tmp/launcher-final-528a-*`; retries corrected stale/moved selection handling and child-retention assumptions, not production code.
- Only test-owned hosts/processes terminated. No installed launcher, push, landing, native credentials or model calls used.

## Strengthened real-compiler rerun

After final evidence review, the harness was changed to append an invalid Go declaration to the private candidate source and require the real Go compiler's `syntax error` diagnostic naming `development_provider_fixture.go`. The wrapper no longer injects failure. The busy-refusal assertion now explicitly requires both before and after pending counts to be zero.

Main-agent execution of `go test ./cmd/cercano -run '^TestEverydayLauncher' -count=1 -v -timeout=10m` with the same outer credential-disabled/offline isolated environment passed both tests (58.240s package result). All compiled journey assertions passed again, including actual build failure preservation. Logs: `/private/tmp/el-mcnw2t5f/real-compiler-acceptance.log`; private artifacts: `/private/tmp/el-mcnw2t5f/real-compiler-acceptance/`. This rerun supersedes the original negative-build mechanism, not its other lifecycle evidence. No real-provider inference, native credentials or installed-launcher change was performed.
