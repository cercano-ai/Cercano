# Prepared-command executor boundary (offline fixtures only)

The repository contains no checked-in real-cloud run script from the private
observations. The reusable checked-in process seam is `Terminal` in
`efforts/dev-host-generations/everyday-launcher-compiled.py`; its `run()` remains
the separate synthetic compiled rehearsal and was **not** run or converted into
a real-cloud launcher.

## Reproduction and implementation

Before the fix, running the new `test_acceptance_execution.py` suite failed at
the real harness boundary: `Terminal` could not accept a prepared command and
artifact checks (`unexpected keyword argument 'artifact_sha256'`). Existing
preparation tests bypassed this seam by invoking `subprocess.run` themselves.

`Terminal(prepared.cli, log=private_log, artifact_sha256=reviewed_hashes)` now
launches the descriptor's exact argv with **replacement** `env=command.env`.
An additional env argument is refused. `run_prepared(command,
artifact_sha256=reviewed_hashes, timeout=seconds)` reuses the same checks for a
bounded non-PTY launcher, direct-host, or CLI call. These are alternative roles,
not instructions to spawn all three or duplicate a launcher-owned host.
Both launch paths check normalized absolute argv, reviewed SHA-256 of the
executable and absolute file arguments, executable permission, and the existing
ledger immediately before spawn. Digests must be independently reviewed, not
computed by the executor as an implicit approval. Checks are point-in-time,
not race-proof filesystem confinement or validation of Go build tags/signatures.

Read-only preflight uses the actual `acceptanceLedger` JSON fields from
`source/server/internal/llm/responses/acceptance_run.go`: `started`, `attempts`,
`input_tokens`, `output_tokens`, `halted`, `active`. It refuses missing/unsafe,
halted, future/expired, exhausted or unresolved state. Launch preflight refuses
**any** nonempty active map (more conservative than HTTP admission, which
refuses expired active deadlines). No ledger or lock is created, rewritten,
reset, copied or unhalted. The existing Go HTTP boundary remains the locked
aggregate authority with 40 attempts, 120-second request deadline, 30-minute
original window, 300,000 total/30,000 output admission thresholds. No new window
or budget is granted; launch-time checks cannot replace HTTP enforcement.

PTY stop is idempotent and terminates/waits/escalates only its unreaped Popen
child, with bounded waits and descriptor cleanup. Non-PTY timeout/error does the
same. Neither sends process-group signals. Detached hosts/other children still
require the existing harness `OwnedProcesses` exact-root/start-identity cleanup;
no general process discovery/cleanup framework was introduced.

## Exact verification

Pre-fix (expected failure):

```
python3 -B -m unittest discover -s efforts/production-generation-integration -p 'test_acceptance_execution.py' -v
```

Post-fix:

```
python3 -B -m unittest discover -s efforts/production-generation-integration -p 'test_acceptance*.py' -v
```

18 tests PASS. New tests cover both Popen boundaries, all three non-PTY roles,
fake worker inheritance, hostile ambient keys, artifact/argv/overlay refusal,
ledger refusal without writes/spawn, missing ledger without creation, PTY
reaping/idempotent stop, and non-PTY timeout escalation against a fake Python
child ignoring SIGTERM. All executed children are temporary Python fixtures,
credentials disabled with temporary HOME; no native opener, application, build,
model, credential or network operation was invoked.

## Concrete prerequisites still blocking real execution

- The recorded aggregate ledger is halted/expired. This work deliberately cannot
  make it admissible; fresh authorization/accounting direction is required, not
  a replacement ledger or changed start time.
- Credential acquisition previously reported `missing`. Read-only permission
  does not authorize sign-in, refresh persistence, identity substitution or
  credential writes; resolve that boundary explicitly before native access.
- Independently review actual launcher/host/CLI argv, artifact digests, signatures
  and `realcloudacceptance`-only build provenance, route/profile/model config,
  read-only credential mutation guards, and private logs/directories.
- A separately authorized caller must select one launch role, use the original
  ledger and original deadline, and arrange `finally` cleanup for its terminal
  plus existing identity-checked detached-host cleanup. This does not qualify
  real host/worker inheritance, complete real-cloud A/B acceptance or retirement.
