# Restricted subprocess acceptance verification

2026-10-26. Final combined verification after ce3f61447114, including worker metadata fix 7d624e17d99f and separate synthetic hold check 361ada46f951.

From source/server with CERCANO_DISABLE_CREDENTIALS=1, GOPROXY=off, GOSUMDB=off:

```
go test -race ./cmd/cercano ./internal/worker ./internal/modelhost -run 'TestDevHost|TestPinnedFixtureSubprocessGenerations|TestProtectedLoadedInstanceABHoldAcceptance|TestInitialProfile' -count=1
```

Parent-executed result: PASS, exit 0. cmd/cercano 18.162s; worker 3.739s; modelhost 1.852s.

See compiled-acceptance-scope.md for strengthened assertions and compiled-execution-evidence.md for the reproduced duplicate-profile completion failure and fix. The prior host-side independent-task simulation was replaced by a real pinned subprocess barrier, with PID/build assertions. Foreground persistence on A/B, detached live sibling survival through B startup and unrelated handover, busy source refusal, completion drain with one matching durable notification record, and subsequent release/retirement pass. Synthetic allowed tool execution and explicit forbidden-tool denial are asserted.

Separate hold accounting uses a synthetic loaded instance and real modelhost server/Remote clients, with holds 2 -> 1 -> 0 and manager Stop calls 0 -> 0 -> 1. It is not real loaded weights/inference or compiled-host loaded-model retirement qualification.

Limits: compiled CLI child binaries are not race-instrumented; the harness and worker test executable are. Caller cancellation occurs after foreground EOF while the independent task remains running, not during active foreground streaming or a physical disconnect. No crash/replay exactly-once guarantee, malicious-worker sandbox, manual TUI acceptance, or whole-repository pass is claimed. The full worker suite under globally disabled credentials was reported failing credential-dependent tests; focused isolated tests above pass. Untagged dev-host execution remains disabled. No real credentials, installed applications, live model runtime, external providers, push, or merge were used.
