# Real-cloud A/B result — 2026-10-29

## Evidence and bounded outcome

The later private result `/private/tmp/rcab-final-herj269a/result.json` reports **PASS bounded real-cloud A/B journey**. This document extracts its nonsecret identities, assertions, usage and cleanup only; it does not reproduce prompts, conversation history, environment dumps, authentication artifacts or process command lines. This is a documentation-only review of that result, not a new live run or independent re-execution.

The [earlier credential-refusal observation](real-cloud-observation-2026-10-29.md) remains valid evidence of its separate blocked run. The later result reports `prior_artifacts_unchanged: true` and `old_ledger_unchanged: true`; it does not establish how the earlier credential-acquisition blocker was resolved.

## Generation identities and observed sequence

Both selections used contract `cercano-development-owned-v2`, schema 2, and the same conversation store.

| Generation | Host identity | Build identity |
| --- | --- | --- |
| A | `d554d0751993415f053172fa43dbffec` | `fcabd9ab-e75c-4e34-9bcd-d3a49a019fab` |
| B | `b0e3993fb0b62bce815e1317f8235ad6` | `1676e2e5-5884-4a12-88fd-1fac74201045` |

The result describes B as a real rebuild with a distinct linked version, manifest/build identity and binary digest, using the **same production source**. It records reuse of the existing B generation in place while A was alive, not a new source change during this continuation.

1. A launched and produced the durable real reply assertion `REAL_CLOUD_A_CONTINUATION_READY`.
2. B launched while A remained alive. A then produced `REAL_CLOUD_A_AFTER_B`, establishing continued A inference after B launch.
3. A's client detached at quiescence while its host remained alive. Ordinary resume transferred the original conversation (`f1bc295b35a6f03168e339970ef3cde1`) to B and displayed history. B's control listing showed ownership of that conversation with a new epoch; B produced `REAL_CLOUD_B_ORIGINAL_RESUMED`.
4. A's subsequent control listing had no retained/open owners. The result explicitly records no spare A sessions and says not to release the transferred selection. A's retire control exited 0 with `ready: true`, followed by recorded A process exit.
5. B produced `REAL_CLOUD_B_AFTER_A_RETIRED` on the resumed original conversation after A retired.

These are assertion labels from the recorded successful replies, not copied raw prompts or history. Coexistence and ordered replies are established; simultaneous in-flight generation is not demonstrated by this sequence.

## Usage and limits

The final ledger is **cumulative**, including `prior_usage`; it is not nine new attempts in this continuation.

| Ledger scope | Attempts | Reported input tokens | Reported output tokens |
| --- | ---: | ---: | ---: |
| Prior usage carried into result | 5 | 46,217 | 56 |
| Continuation increment (difference) | 4 | 37,100 | 48 |
| Final cumulative ledger | **9** | **83,317** | **104** |

The result records caps of 40 attempts, 300,000 combined tokens and 30,000 output tokens. Cumulative combined reported usage is 83,421 tokens, below those caps. It records a 300-second budget and elapsed time of approximately 30.06 seconds. Final ledger state is `halted: true` with no active attempts. These are acceptance-ledger counts, not total project spend, billing totals, or main-chat accounting.

## Cleanup and limitations

After the successful A retirement and final B reply, cleanup records SIGTERM to the B development-host (PID 32658) and B worker (PID 32972), with `remaining: []` for the tracked identities. Do not reinterpret this final B process cleanup as evidence of successful B management retirement.

The independent-task exercise was **skipped**: the existing delegated fixture required a synthetic provider and no deterministic safe real-provider fixture had been established. This result does not qualify independent delegated work, settings/MCP inference, full everyday-use parity, or live migration of in-flight work. The demonstrated transfer was ordinary resume/history transfer **at quiescence**, followed by real replies and A retirement. No full parity or live-migration claim is made.

No tests, applications, network calls, credentials or other live data were accessed for this documentation update; only the requested instructions, earlier observation and private result were read. Private result assertions are the evidence boundary here, not an independent audit of their underlying artifacts.
