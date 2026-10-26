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
The final PTY run **ran and passed**: durable history, A version, accepted independent
work held behind a barrier, physical terminal kill while that work was active,
busy release refusal, barrier completion, explicit reconnect, detach/transfer to B,
explicit reconnect in the same running TUI, separate unchanged TUI/B identities,
A retirement, stale A reconnect refusal, and B foreground operation. It does not
replace the whole final integration gate. Earlier archived CLI still rejects
`--dev-host`, confirming the old blocked startup path.

## Gaps, not waived

An initial PTY experiment killed the terminal while the **foreground** synthetic
provider itself was blocked. Release then succeeded: foreground survival is NOT
established (the transport-bound foreground work ended). No server lifecycle or
execution redesign was made to change that observation. The passing harness
instead disconnects while accepted independent work is still blocked, before its
completion, and verifies that work prevents release.

The PTY harness does not separately prove busy-transfer refusal, an unrelated
conversation transfer during another conversation's work, lost-receipt retry,
multiple-terminal refusal, or real loaded-model retirement. Existing direct-RPC
tests are supplementary, not terminal evidence for those cases. No real-model,
manual acceptance, whole-suite green, landing, installation, or merge claim.
Regression-isolation edits and plan.md are separately owned and excluded.
