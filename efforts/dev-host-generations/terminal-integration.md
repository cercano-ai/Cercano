# Minimum explicit terminal integration — 2026-10-26

Development-only startup: `cercano-cli --dev-host /absolute/selection.json`.
This branch runs before normal config loading and auto-launch. Selection JSON:

```json
{"Endpoint":"127.0.0.1:12345","TokenFile":"/private/fixture/token","Identity":{"host":"A","build":"exact-build","contract":"exact-contract","schema":2,"store":"/private/fixture/conversations.db"},"Conversation":"journey","Epoch":"exact-retained-epoch"}
```

Use prepared fixture identities and private per-host tokens, not the illustrative values.
Startup requires an existing retained owner. Existing authenticated pinned-session
APIs check exact host/build/contract/schema and conversation epoch; no discovery,
launch, implicit acquisition, or cross-host retry is introduced.

`/host` documents controls. `/host detach` retains ownership, `/host reconnect`
explicitly revalidates the current selection. Management uses existing lifecycle
RPCs: `list`, `acquire`, `release`, `retire`, `transfer <destination.json> <unique-id>`,
and `claim <destination.json> <same-id> <receipt>`. Transfer/release require detach;
other live attachments and busy work remain server-enforced refusals. A successful
transfer selects B but requires explicit reconnect. The running executable and
TUI version remain unchanged. Preserve receipts from failed/pending transfers;
do not substitute acquire for claim retry. Detaching is not a promise that a
foreground request survives transport closure.

For management without attaching:
`--dev-host selection.json --dev-host-action ACTION`, with optional
`--dev-host-destination`, `--dev-host-handover`, `--dev-host-receipt`.
Output includes the RPC response and updated destination selection where applicable.
Persist an acquired epoch from the response explicitly before a later startup.
Pinned mode deliberately limits slash commands to host controls, version, search,
and quit; switching conversations requires a new explicit startup selection.

## Checks actually executed

All commands below used `CERCANO_DISABLE_CREDENTIALS=1`; final runs also used
`GOPROXY=off GOSUMDB=off`.

- CLI: `go test ./internal/ui -run 'TestDevHost|TestVersion' -count=1` — PASS.
- CLI: `go build -o /tmp/cercano-terminal-checked .` — PASS.
- Server: `go test ./pkg/agentclient -run 'TestPinned|TestAuthenticated|TestLifecycle' -count=1` — PASS.
- Server: `go test ./cmd/cercano -run '^(TestDevHostTerminalPTY|TestDevHostCompiledStartup|TestDevHostCompiledExecution|TestDevHostCredentialGate)$' -count=1` — PASS.

`TestDevHostTerminalPTY` builds distinct A/B tagged synthetic binaries and an
unchanged standalone TUI. It seeds private stores, starts an empty synthetic
modelhost, and invokes `terminal-pty.py` against actual PTYs. Child environments
exclude provider/config/credential environment and use private HOME/XDG/TMP.
The extended PTY run **ran and passed twice** with
`CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test ./cmd/cercano -run '^TestDevHostTerminalPTY$' -count=2 -v`.
It holds independent work on X (`journey`) behind the compiled fixture barrier,
physically closes X's terminal, and only then starts B. With Y (`sibling`)'s
terminal open, a separate actual management CLI PTY targets X and asserts the
specific admission-busy message (`retained sessions or work prevent transition`),
not merely a nonzero exit. The existing pinned Y TUI cannot target X through a
slash command; the existing explicit management selection supplies that target.
Y then detaches and transfers A→B through its own TUI while X remains blocked.
Before releasing the barrier, the Go harness uses the fixture store's TaskStore
to assert X is still running. After release it checks the successful A-build result,
cleared notification-pending flag, and exactly one durable completed notification.
Y explicitly reconnects on B in the unchanged TUI; X reconnects on A, detaches,
and releases its retained session. CLI retirement of idle A succeeds, stale A
reconnect fails, and Y runs a fresh B foreground request successfully.

All control assertions use actual CLI processes (TUI or management PTY), not
substituted direct RPCs. Setup acquisition/seeding and read-only durable checks
remain fixture helpers. File barriers and durable observations gate transitions;
no sleep alone is evidence of active work or completion. Raw PTY transcripts and
management output are emitted into verbose Go test output before private fixture
cleanup; the two-pass run was retained at `/tmp/terminal-check.log`.
This does not replace the whole final integration gate.

## Gaps, not waived

An initial PTY experiment killed the terminal while the **foreground** synthetic
provider itself was blocked. Release then succeeded: foreground survival is NOT
established (the transport-bound foreground work ended). No server lifecycle or
execution redesign was made to change that observation. The passing harness
instead disconnects while accepted independent work is still blocked, before its
completion, and verifies that work prevents transfer.

The PTY harness does not prove lost-receipt retry, multiple-terminal refusal,
or real loaded-model retirement. Existing direct-RPC
tests are supplementary, not terminal evidence for those cases. No real-model,
manual acceptance, whole-suite green, landing, installation, or merge claim.
Regression-isolation edits and plan.md are separately owned and excluded.
