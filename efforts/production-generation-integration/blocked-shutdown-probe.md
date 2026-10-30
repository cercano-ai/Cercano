# Shutdown join blocker — 2026-10-27

Status: reproduced lifecycle blocker; no production fix or native credential probe.

## Parent-executed probe

Artifact directory: `/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/shutdown-probe-g9qg_mfh` (temporary overlay sources, overlay.json, result.log).

A Go overlay inserted a channel barrier at the initial cloud-rebuild boundary in the real `startGRPCServerWithLifecycle` function. It reused the successful single-host fixture with disabled credentials, empty cloud configuration, temporary stores, bufconn, fake dependencies and native-opener traps. It did **not** call or simulate a real Keychain backend; the barrier characterizes an unresolved rebuild callback.

Command: `go test -overlay <artifact>/overlay.json -race ./cmd/cercano -run '^TestProductionSingleHostStartupShutdown$' -count=1 -v -timeout=30s`, with `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`.

Exit 0; package PASS (2.114s). This PASS asserts the defect:

- Normal cleanup did not complete during the 100ms blocked observation; `GetConfig` remained usable because RPC shutdown had not begun. Releasing the callback allowed cleanup to finish.
- The late-MCP-failure startup return was held until the same barrier was released.

The observation interval does not prove infinity empirically. Source provides the unbounded property: cleanup and failure defers use unconditional receives on `cloudRebuildDone`; the watcher join is also unbounded. No source files were changed by this overlay.

## Source-supported hazard and contract gap

Read-only tasks `task_be7961d54f1013faeeafb67e12665fa8` and `task_c2be818002e2bb03bdb1da6bef1f2391` identify synchronous `Store.Get`, no context/cancel API, and native adapter `keyring.Get`. A real authorization prompt may therefore hold the rebuild. Actual native behavior was not exercised. The second child's overlay execution failed to launch Go; the parent probe above supplies executed evidence instead.

The existing drain helper has a 10-minute grace and 2-second hard-stop backstop, but the new joins run before it. Cleanup is `func()` with no incomplete status. A simple timeout followed by closing stores, diagnostics, runtime or policy state would permit unresolved callbacks to use closed/retired resources. Moving the wait alone would begin drain sooner but would not bound total cleanup.

A safe bounded result requires separating admission closure/drain from final disposal, retaining the unresolved callback's dependency/authority set until it finishes, and representing incomplete shutdown explicitly. Callers must not interpret that result as successful retirement or permit same-process replacement. Otherwise the process termination/credential isolation boundary would need redesign. This is a shutdown contract decision, not authority to stop existing processes.

Canonical generation activation remains disabled. The successful empty-cloud startup fixture remains useful but is not blocked-credential or active-worker shutdown qualification.

## Separate producer probe

Parent ran `go test -race ./internal/compactiongen -run '^TestLegacyScheduleContextLossAndTimerGap$' -count=1` with the same offline/credential flags: exit 0, PASS (1.492s). It asserts legacy context loss and uncounted timer behavior, not a fix. Two implementation attempts could not launch Go from child tooling; no owned scheduler implementation exists. Parent tooling still runs Go. This is not a user/toolchain prerequisite claim.
