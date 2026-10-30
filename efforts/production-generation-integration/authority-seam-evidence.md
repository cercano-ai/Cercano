# Bounded authority / credential seam

Base: `7be8c983ed60527ebd6e0b219e805283686f0bef` (following shared composition checkpoint bf865c0ed007).

Implemented a pure invocation-local canonical path resolver. Defaults delegate unchanged to config.DefaultPath and conversation.DefaultPath (including its existing override semantics); resolution calls each injected function once and preserves errors. Tests inject temporary fixture paths exclusively. This resolver is **not activated in startup**.

Ordinary production's two credential-opening sites and prepared production now use the same explicit dependency type. Ordinary defaults remain the real opener; prepared preflight still precedes opening and its read-only guard/freeze remains. No new enabled production path, legacy opening from prepared paths, or permission-package edit is part of this slice.

Tests include an enabled owned-child memory-store positive control, read-only mutation refusal, a preflight native-shaped opener trap, resolver failure propagation, nil dependency rejection, and the existing injected prepared graph tests. Child environment is explicitly minimal (no inherited provider credentials), with bounded lifetime. No test calls the canonical defaults resolver or enabled real credential opener. The trap proves preflight ordering, not universal native isolation across enterprise/runtime/provider graphs.

Validation (source/server, offline GOPROXY=off GOSUMDB=off, outer CERCANO_DISABLE_CREDENTIALS=1):

- `go test ./cmd/cercano -run 'TestProduction|TestDevelopmentProduction' -count=1` PASS.
- `go test -race ./cmd/cercano -run 'TestProduction|TestDevelopmentProduction' -count=1` PASS (includes race-instrumented owned test child).

## Exact next boundary

Ordinary startGRPCServer currently opens enterprise and connects runtime before conversation.DefaultPath/openAgentConversationStore. Its subsequent recap/compaction setup, server construction, config persistence/watchers, telemetry, credential migration and MCP/worker setup interleave side effects. Safely consuming a resolved authority object requires splitting those stages and installing compatibility preflight ahead of **all** mutation-capable openers, not just changing path strings. Ordinary callers also load config before this function. This slice deliberately does not reorder those effects.

Prepared composition continues to pass the exact store supplied by NewPreparedDurableServer to the agent, with existing exact lifecycle-store validation. Generalizing the ordinary constructor to accept that same supplied lifecycle/store is still pending; the pure paths seam must not be used to reopen it through legacy Open. Prepared snapshot paths and config freeze are unchanged pending later gates.

Phase 1 credential seam and opener/child isolation tasks remain in progress, not complete. Plan status updates remain in the shared working tree (the plan had pre-existing/concurrent edits); this checkpoint excludes that shared file and all concurrent permission changes. No installed launcher, live DB, native credentials, existing process, merge, or push accessed.
