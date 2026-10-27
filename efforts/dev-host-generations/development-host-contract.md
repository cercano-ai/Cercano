# Production development host command

**Implementation status: incomplete.** The command boundary, authenticated record
publication/lookup, and invocation-local factory exist. The production factory
currently refuses startup: the provider/tool graph, protected runtime, and
exact-owned independent child worker bridge are not installed. Do not advertise
this as a working production host or an end-to-end launcher acceptance result.

Entry: `runDevelopmentHost(ctx, args, out)` (dispatch is owned by orchestration).
Flags: `--state`, `--generation`, `--host`, `--token-file`, `--record`, `--runtime-root`, `--listen 127.0.0.1:0`.
All paths are absolute, canonical (no symlink aliases). State and runtime directories and token/record parents are private. `state/config.yaml` is explicitly user-provisioned. Store preflight uses `DevelopmentCoexistenceDeclaration()` before dependency construction. No legacy startup entry is used.

Record is JSON `{Endpoint,TokenFile,Identity,Conversation,Epoch}`; Identity is proto LifecycleIdentity, Conversation/Epoch initially empty. Publication follows an authenticated identity probe. Peer dialing resolves only private `state/hosts/*.json` records and verifies authenticated identity.

Runtime availability is fail-closed: dependency construction must provide the full prepared production graph and pinned worker. No fixture execution or in-process fallback is permitted. The command's dependency factory is per invocation (not an environment bypass).
