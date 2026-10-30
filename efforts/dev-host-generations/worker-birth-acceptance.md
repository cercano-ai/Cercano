# Remaining worker-birth acceptance gap closed

Candidate tested: `6adc9a08f7601d254c8df46df1a459ad81c387ab` (parent `23c893384763`). No production lifecycle feature was added. The only ordinary-build hook is a no-op; child-entry evidence requires both `developmentfixture` and `cercano_development_acceptance` tags.

## Retained-trace reproduction (no old run repeated)

`worker-birth-regression/retained-worker.jsonl` preserves the old trace from `/tmp/ca-x3_9e_r5/barrier/worker.jsonl`; `retained-boundary.json` includes its digest and retained B selection observation. PID **98189** handled a turn at **1793244802071446000**, before the B selection file timestamp **1793244810902881359**, then handled turns after it (first **1793244813616799000**). These are `worker-turn` records, not startup records. The old later-turn assertion therefore accepted reuse. The regression test demonstrates that old condition passes while the new assertion rejects it. The old selection filesystem timestamp is only retained ordering evidence, not an exact publication or process-birth timestamp.

## Exact committed candidate execution

All commands used outer `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`:

```
python3 efforts/dev-host-generations/test_worker_birth.py
# 6 tests, PASS: retained reuse, missing startup, pre-B startup,
# startup after first turn, reused PID despite startup, genuinely new child.

python3 efforts/dev-host-generations/everyday-launcher-compiled.py \
  --repo . --evidence efforts/dev-host-generations/worker-birth-6adc9a08-evidence
# exit 0, 46.26s, all compiled assertions PASS

# cwd source/server; additionally CGO_ENABLED=0
go test -tags=developmentfixture,cercano_development_acceptance \
  ./cmd/cercano -run '^TestWorkerArgs$' -count=1
# PASS
```

`worker-birth-6adc9a08-evidence/compiled.json` records the exact candidate SHA, isolated source snapshot and tree, pre-B worker events/set, B-publication observation boundary, startup and first-turn records, and all original acceptance steps. The unstarted conversation was retained by ordinary A launch before B, with no prior turn/worker. Its first turn after B naturally spawned a child; no process was killed to obtain it.

- Pre-B worker PID set: **[99039]**.
- B published and ordinary launch completed; observed boundary: **1793245235666478000**.
- New A child **PID 99234**, parent **99037**, child-entry `worker-startup`: **1793245235774379000**.
- Same child's first `worker-turn`: **1793245235805798000**, conversation `2a00e80103e0657631f25175b327da61`.
- Thus boundary < child entry < first turn, and PID 99234 was absent from pre-B worker set. Startup is emitted at `runWorkerMode` entry, not in StartTurn.
- Executable: `/private/tmp/ca-nb0d3m8f/state/generations/f32b83171ba417acf54913833c36b77759e854b1b2c45201394eba71b1634028/cercano`.
- Executable SHA-256: `39a291f066198522af6c1f680a3d3df64f3ed140216d1aae6c3e0fbfbedfa46d`.
- A manifest SHA-256: `b446402df00c520a3013f66687e9cb17690881b5ed7a7e476a3a9fa3848bb2aa`; A build ID `83b171d2-356e-49f9-be91-f8e96bd15387`.
- B generation `ca384e8301d6eb90547792ea2da4d615a862ddb5cc2ac28e1c0de4915e7d320b`; B build ID `03884967-f939-4070-8331-70ec01e16e87`. Both manifests are exported. Assertions require worker path/digest/manifest/build/generation to match A and executable digest to differ from B.

The original independent task was already running before B publication. Original A foreground after B, A client exit while task remains active, ordinary B `/resume` busy refusal with zero pending incoming transfer, task completion with exactly one notice and no deferred transfer, physical B client restart, ordinary `/resume` of original A history, and durable new B turn all passed. `synthetic-reopened-rows.json` exports all 12 synthetic rows from the reopened original conversation, not merely the two marker rows.

## Durable artifacts and limits

All evidence is under `worker-birth-6adc9a08-evidence/`: `compiled.json`, `worker.jsonl`, `provider.jsonl`, both manifests, build log, four PTY captures, synthetic reopened rows, routing config, and synthetic host logs/metadata. Private token contents are not exported. The retained private runtime root is `/private/tmp/ca-nb0d3m8f`.

This is isolated compiled production orchestration with fully injected memory/non-dialing provider/runtime dependencies, not native inference or a native provider integration test. Boundary is an observation after successful B publication/launch (conservative), not publisher instrumentation; startup is child-entry evidence, not kernel process-creation instrumentation. The isolated source snapshot lacks VCS metadata in the binary; exact executable/manifest digests and captured build information bind identity. No live user processes, native credentials, live data, provider network, installer, activation, push, or merge were used. Only invocation-owned synthetic processes are cleaned up. Git work used explicit-path checkpoint directly because the bounded agent instructions forbid further delegation.
