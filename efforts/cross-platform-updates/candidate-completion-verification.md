# Owned child completion and candidate-health boundary

Native run: https://github.com/cercano-ai/Cercano/actions/runs/38030898090
Source 0d256b31b9c9e7ad196c288dd0ca94155db873fd. Windows/Linux/macOS pass.

The launcher's existing sole reaper now retains its result. A process-local
Completion receipt is issued only after that reaper confirms termination.
Cancellation/timeouts/unknown waits produce no receipt, and waiting never signals
the child. A receipt matches the exact launch handle, not a reusable PID or an
executable path. Nonzero exit is completion, not successful health. It proves
nothing about descendants or operation association and cannot be reconstructed
from a persisted PID after utility restart. Actual owned echo-child fixtures on
Unix and Windows verify the receipt; pure tests cover cancellation, multiple
observers, unknown status and matching. No recovery removal calls are enabled.

Source inspection found a high-risk boundary before launching health candidates:
cmd/cercano/main.go's agent subcommand loads the default config and calls
runServerMode/startGRPCServer. That path opens the real conversation store, starts
retention, invokes worker.ReapOrphanWorkers, opens the OS keychain, and can migrate
legacy inline keys into the keychain and rewrite config. A binary pointer rollback
cannot be assumed to reverse those effects. No normal agent was run as a probe.

Pending choice: recommend an isolated, private preflight mode in the existing
binary, followed by separate normal readiness under explicit user-state
compatibility rules. Alternative: normal-startup-only health restricted to releases
with verified backward compatibility. Preflight alone must never count as real
configuration health; unverified post-startup state changes forbid automatic
rollback promises. This choice is awaiting approval before candidate startup and
selection removal are connected. No new service, listener or shipped binary is
proposed.

## Superseding approval and bounded offline preflight (2026-10-22)

The user approved the isolated-preflight strategy; see the existing spec's
startup-safety section. `__internal-update --preflight` now uses the earliest
private dispatch. It accepts no extra tokens and emits a constant JSON document
under 512 bytes. Exit zero means only entrypoint reachability/report delivery;
health is not checked and persistent-state compatibility is unknown. Write
errors/short writes fail. No normal initialization, listener or child launch is
performed. Updater capability reporting remains unchanged and execution false.
No production caller invokes this mode. In particular, do not probe an unknown
old binary using private argv; safe capability discovery remains future work.

Local macOS verification passed with GOPROXY=off and GOSUMDB=off:
- `go test ./cmd/cercano -run 'Test(CandidatePreflight|MainSubprocess|MainWires|DispatchPrivateUpdate|UpdaterProtocol)' -count=1 -timeout=120s`
- Same focused suite with `-race`.
- `go vet ./cmd/cercano`

Tests cover strict/duplicate/extra/equals/locator arguments, oversized rejected
input, bounded output, runner noninvocation, and output failures. Real-main
subprocess fixtures isolate HOME/XDG/AppData, assert no files created, enforce a
deadline and reap the exact child. Wiring tests establish dispatch before
ordinary startup; this is not an OS sandbox or network-packet isolation claim.
No native Windows/Linux execution, live agent/install actions, push or merge.

Normal activation remains unavailable. Explicit persistent-state compatibility
evidence, owned launch orchestration and state-safe rollback with exact candidate
completion remain gates. Preflight does not establish any of them.
