# Follow-up verification — 2026-10-26

## Tested code

Feature: `feat/dev-host-generations`, code HEAD `9004eddd850e93159c8ef45693dd1cd6ecfbb258`. This supersedes the failed gate in final-integration-gate.md, not its historical observations. Conditional local reconciliation/landing remains a subsequent operation, not established by this document.

Repairs include mock interface/credential-policy coverage; scoped fake credential tests and bounded login waits; AWS fixture input isolation; per-worker memory secret injection with authoritative nil/no-fallback tests; config-policy-based prepared persistence freeze; authenticated pinned terminal controls; stale UI subscription generation rejection; and pinned local UI configuration-write refusal.

## Checks

Independent verification task `task_eee62743715b975b8f0ba5ce1af1a6dd` ran with an explicit replacement environment, private HOME/XDG/TMP, GOENV=off, GOPROXY=off, GOSUMDB=off and outer CERCANO_DISABLE_CREDENTIALS=1. No inherited provider credentials or live integration opt-ins. Scoped enabled tests use synthetic dependencies; the five repaired production-tool fixtures inject memory stores. Opener probes ran first under disabled admission.

Artifacts: `/tmp/cp-59arkhbr/` contains verify.py, env.json, commands.jsonl, results.json, complete logs, revisions, skip inventories, binaries and SHA256SUMS. Parent read results.json and verify.py directly.

| Check | Result |
| --- | --- |
| Worker injected-opener/nil-result probes | PASS |
| Server go test ./... -count=1 -json | PASS, 117 packages |
| CLI go test ./... -count=1 -json | PASS, 10 packages |
| Server and CLI builds to private output paths | PASS |
| TestDevHostTerminalPTY -count=2 -v | PASS twice |
| Full worker/persistence/config/server package race checks | PASS |
| Full CLI UI package race check | PASS |

Built-in suite skips remain: 35 server and one CLI, covering live opt-ins, platform/helper cases, an explicitly aspirational semantic-collapse case, network-normalization timeout and catalog terminal driver requirement; exact lists are retained. No test failure was hidden with a new skip.

The initial verification at `/var/folders/9h/srsrfxw55fs2x54j3w7syc700000gn/T/cercano-preland-fbtdarnm/` failed TestSpawnWorker_ProcessOutlivesTurnContext with Unix socket `bind: invalid argument` and readiness timeout under the long temporary root. All checks were rerun without source changes under the short private path above and passed. Unix socket path-length sensitivity remains an observed limitation, not a fixed regression.

## Actual terminal evidence (automated synthetic fixtures)

The expanded PTY harness uses real compiled A/B agents and an unchanged standalone terminal UI, private stores, synthetic restricted providers/tools, and a protected empty modelhost. It physically terminates the X terminal while an accepted independent worker is barrier-blocked, starts B, verifies exact busy-X transfer refusal through management CLI in a PTY, transfers quiescent Y via its TUI while X remains running, verifies successful durable X completion and exactly one notification, reconnects/releases X, retires A, rejects stale-A connection, and verifies B remains usable. Reads of final durable task state supplement—not replace—terminal controls. Transcript output is retained in pty-repeat.log.

This is automated actual-terminal acceptance, not a human manual check. Foreground transport-bound request survival is not promised: an earlier foreground-disconnect experiment ended that request. Independent-task active-disconnect survival is the verified contract.

## Credential incident and limits

See credential-test-isolation-incident.md. Earlier temporarily enabled worker tests may have reached the native opener; success and credential access were not instrumented and remain unknown. Later repairs and passing isolated tests do not retroactively erase that incident. Read-only review `task_7460ebda7cb03613e87fc0c0c26720df` confirms the five repaired fixtures' injected paths and no fallback, but is not blanket clearance to globally enable the suite. Other default worker-stack tests must retain outer credential disabling.

No real-provider/model inference or real loaded-model retirement evidence was collected. Synthetic modelhost participation/hold accounting is not real loaded-weight qualification. Compiled child binaries are not race-instrumented by parent go test -race. Linux/Windows native acceptance and crash recovery remain outside this evidence.

No push, live installation changes, or live-host restart is part of this verification.
