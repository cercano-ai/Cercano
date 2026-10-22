# Bounded real configuration experiment — FAIL (effective defaults drift)

TEST ONLY, 2026-10-22. This is actual `pkg/config.Load` / `config.Save` evidence,
not a production compatibility declaration, artifact qualification or startup test.

## Exact inputs and reproduction

- Approved prior stable v0.25.0: `58befd2801dd6f3d53438e8c8bfc74d937a9e811`.
- Candidate: `e9496fb6c35df349ab7850016fce1c67c37e0589`.
- Local Git objects archived independently with `git archive ... source/server`.
  No checkout mutation or compilation of mutable production source. The only
  extra source inside each temporary module is this test harness.
- Toolchain: Go 1.26.3, darwin/arm64. Python 3 runner, standard library only.
- `results.txt` records exact commands, exits, runner/source SHA-256 hashes and
  both compiled harness SHA-256 hashes. Binary hashes are per-run (temporary
  build paths vary), not published artifact hashes.

Executed from repository root:

```sh
COMPAT_GOMODCACHE=/Users/bryancostanich/go/pkg/mod \
  python3 efforts/cross-platform-updates/fixtures/compatibility/configuration/run.py \
  > efforts/cross-platform-updates/fixtures/compatibility/configuration/results.txt 2>&1
```

Runner exit **1**, deliberately retained. Both archived module builds pass.
Both focused suites pass: `go test -mod=readonly ./pkg/config -count=1
-timeout=60s` (prior 0.628s, candidate 0.626s). Passing unit tests do not override
cross-version comparison failures. On another machine explicitly supply an
already populated module cache; offline missing dependencies are failures.

## Isolation and actual subprocess behavior

The runner constructs an allowlisted environment, rather than inheriting API
keys or provider settings: temporary HOME, all four XDG directories, USERPROFILE,
APPDATA, LOCALAPPDATA, TMPDIR and GOCACHE; explicit GOMODCACHE;
`GOENV=off GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off`; disabled
system/global Git config. PATH is inherited for tool lookup. No normal agent,
credential service, native keychain, OAuth login or network client is invoked.
The harness imports only config and standard-library packages. URLs/model/account
names and the diagnostic secret are synthetic; `.invalid` endpoints are never
contacted. Config paths and expected snapshots are explicit temporary paths.

**This is not zero-subprocess execution.** Besides Python's Git/Go/harness
children, production `Defaults()` calls platform memory detection. On Darwin
`memory_darwin.go` executes `sysctl -n hw.memsize` via `os/exec`; Load, Save's
baked catalog handling and package tests can call Defaults repeatedly. It is
not stubbed or blocked here. Observed RAM-dependent defaults in this run are
MaxSeqLen 32768 and PAMemoryFraction 0.35. Linux's implementation reads
`/proc/meminfo`, and other platforms return unknown memory (not tested here).
This is trusted-code environment isolation, not a network/OS sandbox or syscall
trace. Harness process-group timeout is 20s, builds 240s, focused suites 180s
(with Go's own 60s timeout), other commands 120s. Temporary sources/configs are
removed on exit. Save's normal file permissions/atomicity are not hardened here.

## Assertions and results

For each empty/default and populated scenario:
prior Load + seed + Save → prior seed-check → candidate seed-check → candidate
Load/modify/Save → candidate verify → prior verify. Failed reads do not prevent
subsequent writes/readback evidence. Expected snapshots originate **before**
prior Save, so prior same-version loss is not silently accepted.

Populated settings cover port, local endpoint/runtime, locus and worker modes,
idle timeout, capture-reasoning flag, two cloud profiles and their explicit
provider/backend/route/base URL, active and ordered backup accounts, sparse cost
tier and image overrides, runtime-specific everyday model override, and explicit
8192-token llama context. Remaining fields retain prior defaults. Candidate
changes port to 50992, idle timeout to 123, shutdown-on-last-client to false and
primary economy override to `synthetic-candidate-economy`; all other fields must
remain equal. Full loaded Config objects are compared, not merely successful
parsing, including nested settings and effective ModelProfiles.

- Prior seed-checks and prior post-candidate-write verifications **PASS** in
  both scenarios. The chosen candidate edits remain readable by prior.
- Candidate seed-check and candidate post-write verify **FAIL** in both
  scenarios, solely in `ModelProfiles`. All other compared fields match.
- Candidate's baked OpenAI defaults differ: economy `gpt-5-mini` vs prior
  `gpt-6-luna`, standard `gpt-6-astra` vs prior `gpt-6.1-sol`; ChatGPT economy
  and standard become `gpt-6-astra` instead of those prior values. Candidate
  lacks the prior baked Mistral and Cerebras vendor tables.
- Production Save intentionally strips baked catalog entries, and Load restores
  its own version's catalog. Thus this is **effective default/model-selection
  drift**, not observed loss of the explicit settings exercised here. Prior
  reload restores its own defaults. This fixture does not label every default
  change a serialized-format incompatibility, but cannot promise preservation
  of all valid prior effective semantics. The strict comparison remains failing;
  no catalog field was excluded to manufacture a pass. Production policy or
  remediation requires approval; no production fix was attempted.

### Fixture development evidence

`results-initial.txt` and `harness-initial.go.txt` preserve the initial run and
harness. It also reported nil-vs-empty slice differences for CloudProfiles and
both runtime ExtraArgs, even on prior → prior Save/Load. YAML materializes these
empty lists; JSON preserves their Go representation difference. The final
harness normalizes only these three zero-length lists to nil for semantic
comparison, preserving all nonempty contents. No defaults or settings values
are normalized or removed. The real ModelProfiles failures persist. The final
runner additionally gives each diagnostic writer independent raw input, avoiding
prior Save dropping unknown keys before candidate can test its own parser.

## Separate diagnostics — not promised prior-valid semantics

Unknown-field and legacy inputs are deliberately separate from the saved-prior
round-trip contract. Each is fed independently to prior and candidate Load/Save,
then prior reads/writes the resulting file. Full synthetic loaded/saved values
are printed in the log. All eight diagnostic invocations exit **0**:

- Unknown `fixture_future` mapping is accepted and discarded by Save in both
  versions (asserted absent after Save). This is **not unknown-field retention**.
- Legacy `local_runtime`, `local_model`, `locus_mode: local_only` normalize to
  ollama, an everyday override `synthetic-legacy`, and `open_only` (asserted).
  This does not qualify every historical migration, Meridian/subscription
  transition, deprecated cloud secret, unknown nested field or malformed input.

## Limits / blocker

Bounded fixture complete with a negative effective-semantics result. No broader
compatibility guarantee, credentials/keychain qualification, all-provider model
resolution guarantee, native Windows/Linux proof, live startup, crash/concurrent
write safety, artifact binding or activation/rollback authorization follows.
No compatibility IDs, signing, publication, push or production modifications.
Original conversation evidence is preserved. The effective defaults drift is
recorded for review rather than fixed or waived.

Additional fixture checks: Python AST parse passed; `git diff --check` passed.

## Approved bounded remediation — 2026-10-22

User explicitly approved: **Preserve v0.25.0 provider defaults and resolution
behavior.** This executes the existing approved effort, not a new plan or waiver.
Inspected exact baseline source at
`58befd2801dd6f3d53438e8c8bfc74d937a9e811`, not just the previous summary.
Restore OpenAI/ChatGPT tables, Mistral/Cerebras tables and host inference, and
Mistral implicit vision; explicit provider/backend, tier and image precedence
remain intact. No schema, updater or Meridian migration changes.

Before production edits, new `TestBaselineProvider*` probes failed (exit 1,
0.390s): all four provider tables/resolutions and Mistral implicit vision drift
were reproduced; explicit text/image override assertions did not fail.
`regression-before.txt` retains the failure output. Existing model-profile
expectations were restored to the exact baseline tests (no assertions removed).

After the fix, offline isolated tests passed (all exit 0):
- `go test -mod=readonly ./pkg/config -count=1 -timeout=60s`: 0.677s.
- Same suite with `-race`: 2.003s.
- Focused `TestBaselineProvider|TestResolveCloud|TestExplicitProvider|TestRetiredOpenAI`
  with `-v -count=1 -timeout=60s`: 0.196s.

`regression-after.txt` records commands and results. Tests used temporary
HOME/XDG/cache paths and an existing module cache with network module lookup
disabled. No normal startup, provider calls, real configuration, databases or
keychains were used. `git diff --check` passed. Committed-candidate compatibility
rerun is pending below; the earlier failure above remains historical evidence.
