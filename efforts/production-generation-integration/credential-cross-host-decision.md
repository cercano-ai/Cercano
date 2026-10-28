# Cross-host credential logout ordering — prompt fencing approved

## Approval update — 2026-10-28

The user approves prompt local logout with durable stale-operation fencing for existing credential identities across participating hosts. Stale login/refresh commits must validate and publish atomically relative to logout. Issuer calls stay outside short shared commit exclusion; competing refreshes are separately serialized. Cached credentials, partial failures, restart recovery and incompatible older writers must be explicit. Logout succeeds only after local credential removal and durable fencing; it does not promise remote revocation or cancellation of in-flight issuer calls.

Implementation is limited to this worktree and isolated fixtures: no native credentials, live data, migration, installer, new service, activation, push or merge. Credential-enabled test scopes require fully injected memory stores/fake issuers, without child/native fallback; outer execution uses CERCANO_DISABLE_CREDENTIALS=1 offline. Existing config apply/retry limitations remain separately unresolved. The historical alternatives below are superseded by this approval, not reopened. Any genuinely new credential security/compatibility representation decision must be reported before implementing that representation.

2026-10-28. Approved Phase 3 already requires preventing late refresh from resurrecting logout. This note does not reopen that requirement; it records a narrower security/availability choice about an in-flight external operation. No credential coordination implementation was performed.

## Observed evidence

Source audit task task_a0433766d4c7f1464f5a208d6e005a6b found per-Service profile maps/mutexes/generations, plain credential CRUD, and no shared durable generation/tombstone. It did not locate the repo-root spec from its server-scoped working directory; its product-contract uncertainty is superseded by the next probe's reading of the actual approved spec/plan.

Diagnosis task task_9356ad0f1e33d8683f62191b080bfda7 read the approved spec/plan and reported a temporary memory-only barrier probe reproduced resurrection 10 times under -race. Two Service objects shared secrets.NewMemory. A staged a refresh and waited; B.Delete succeeded and backing state was absent; releasing A allowed its unconditional Set to restore the credential. Passing diagnostic tests assert the defect, not security correctness. The test was removed. Outer credentials were disabled; a local nonparallel enabled scope used only explicitly injected memory store/package-local fake transaction resolver. No auth HTTP/native backend/live user data was used. This is two-service evidence, not multi-process native qualification.

Relevant source: hostsvc/credentials/service.go profile generation and flight (24–69), Delete local invalidation (138–155), snapshot acquisition (214–230), local-generation commit check and Set (257–271). Auth envelopes contain tokens/expiry/account identity, not durable refresh uncertainty or generations. secrets.Store exposes ordinary Get/Set/Delete/List, not conditional commits.

## Historical ordering decision — superseded by approval above

Should logout wait behind an admitted cross-host refresh, or supersede it without waiting for the issuer?

- **Serialize logout behind refresh:** shared identity operation exclusion spans external refresh and credential commit. Logout waits or returns explicit busy/pending; it does not claim success early. Lower coordination complexity, but an uncooperative issuer can delay logout. Ambiguous token rotation still needs explicit handling; a lock is not a crash-recovery solution.
- **Prompt logout with durable stale-operation fencing (recommended):** invalidate the identity's operation generation before deletion, reject delayed refresh/login commits, and serialize competing refresh operations separately. More complex persistent coordination and compatibility/crash tests, but logout need not wait for an external issuer. This cannot guarantee immediate revocation of already-issued remote tokens, and native backend calls may themselves block.

Both require participating hosts, fresh reads under coordination, conservative external-rotation ambiguity handling, and no secret-bearing notifications. Neither option authorizes a new service, second credential authority, changing existing credential profile identities, or live native access. Specific durable representation/location and compatibility implications require source-grounded review before implementation; none was selected by the audit.

## Concurrent completed work

Watcher slice c0e4e4d7c31c adds two-Server callback convergence and saved/not-applied outcome tests. Parent reran (offline, CERCANO_DISABLE_CREDENTIALS=1): go test -race ./internal/server ./internal/hostsvc/providers -run 'TestCrossHostConfig|TestConfigApplyCounterexample|TestReloadConfigFromDisk|TestStartConfigWatcher|TestUpdateConfig|TestRemoveCloudProfile' -count=2. Server passed; providers had no matching tests. Full suites were not repeated after this slice. Failed application can still retain old provider graph, and identical baseline reload skips retry; applied-revision/admission reconciliation remains incomplete. Canonical activation remains disabled.

## Current-run reproduction and representation gate

Baseline: clean `feat/production-generation-integration`, `e340ea85df84a2a28c5d31e6a4c12eab207e4ccc`. Read root AGENTS, approved spec and plan before changes. The credential serialization task was marked in progress through `plan_set_status`.

Retained `TestLegacyTwoServiceResurrectionProbe` in `source/server/internal/hostsvc/credentials/cross_service_test.go`. Two Services share `secrets.NewMemory`; A's package-local fake resolver waits on a barrier; B.Delete returns success and Get reports absence; releasing A publishes `rotated`. This deliberately passing diagnostic asserts the defect, NOT corrected security behavior. Before any production fix, it passed 10 times under race instrumentation; after formatting/cleanup bounds it passed another 10 times. No production fix has been made.

Exact command (server directory):

```
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off go test -race ./internal/hostsvc/credentials -run '^TestLegacyTwoServiceResurrectionProbe$' -count=10
```

Only that named test executed. Its nonparallel enabled scope injects memory and a package-local fake resolver; no HTTP issuer, child, native opener, or fallback is reachable from this fixture. No full-suite/native/multiprocess claim. An initial tool invocation using a relative cwd failed before Go started; the absolute-worktree invocation succeeded.

### Source-grounded seams (proposed, not added APIs)

- `secrets.Store` (`internal/secrets/secrets.go:20–30`) is CRUD only. Native `Set`/`Delete` (`:89–93`) cannot atomically compare a fencing revision. `service = "cercano"` and profile keys remain the identity; no generation-local credentials.
- `Service.acquire`/`run` (`internal/hostsvc/credentials/service.go:201–285`) already separate staged issuer output from publication. Replace the per-Service-only commit predicate with shared identity admission and final validation/publication under the same short commit exclusion. Validate even when nothing was staged: a previously loaded fresh token must not escape after logout wins the final admission check.
- `BeginLogin`/`LoginAttempt.Commit` (`login.go:23–85`) need captured shared revision and conditional commit, rather than merely `p.login == a`. `Set`, `Delete`, `ReplaceStore`, Get and List need explicit coordinated semantics; an uncoordinated backend must not silently acquire coexistence capability.
- Both auth Sources load then refresh then Save (`anthropicauth/source.go:73–126`, `chatgptauth/source.go:70–129`). Add an admission callback immediately before the actual issuer request, so a durable pending-rotation marker is not written just for a cached-token read. Hold separate refresh exclusion, reread after admission, release commit exclusion before calling the issuer, then validate/publish under commit exclusion. No blind replay after uncertain refresh.
- Reuse `filelock.Lock(ctx, path) (func(), error)` (`internal/filelock/filelock.go:46`) for distinct per-identity refresh and commit lock files. These are advisory locks, not durable generations or legacy-writer exclusion. Stable identity mapping must not depend on a generation's config override.
- Candidate narrow coordinator operations: `Snapshot(ctx, identity) (Revision, error)`, `WithCommit(ctx, identity, func(State) error) error`, and `WithRefresh(ctx, identity, func() error) error`. These are design signatures only, not exported or implemented interfaces. A complete backend must validate revision and execute credential publication inside WithCommit, not return a checked boolean for a later Set.

### Decision: where does the durable fencing state live?

This is a concrete security/compatibility boundary under the spec's final infrastructure paragraph, not another choice about logout timing. Existing auth JSON has only access/refresh/expiry/identity (plus ChatGPT account ID): `anthropicauth/anthropicauth.go:75–100`, `chatgptauth/chatgptauth.go:43–69`. Both decoders ignore unknown JSON fields, and legacy Save re-encodes only known fields. Merely adding a revision field therefore lets old writers silently erase it. Ordinary API keys also remain raw store strings. A durable representation has not been approved by the historical note or selected here.

Decision: should durable fencing use an existing-identity native envelope or a security-critical non-secret filesystem journal?

| Axis | Versioned native envelope at existing profile key | Non-secret filesystem journal alongside native credentials |
|---|---|---|
| Representation | Version, random revision, active/logged-out/pending state, operation ID and existing payload; logged-out record contains no secret | Version, random revision, status and operation ID only; native payload format unchanged |
| Cost estimate, not measured | ~400–700 implementation lines across 5–8 files; similar fixture-test volume; existing filelock, no new dependency | ~500–900 implementation lines across 6–10 files; similar fixture-test volume plus filesystem faults; existing filelock, no new dependency |
| Security/compatibility risk | Incompatible native payload format; legacy writers can erase tombstones; retaining non-secret tombstone changes physical Delete semantics | New filesystem trust dependency can roll back/lose fencing independently of native store; missing journal cannot mean fresh generation |
| Outcome | Keeps authoritative fencing and secret payload under the existing native backend protection/identity | Preserves native payloads and actual native item deletion; journal is coordination only, not a second secret store |
| Recovery/test burden | Pending state blocks reuse after uncertain rotation or publication; test malformed versions, failed writes and missing tombstones | Pending state plus write-ahead ordering and sync durability; test every native/journal partial failure and missing/corrupt journal |
| Runtime side effects | Additional native metadata accesses may prompt/block; fresh reads remain necessary | Additional short file locks and fsync; native deletion can still block; permissions/symlink checks necessary |
| Older writers | Explicit exclusion/preflight required before adoption; format alone cannot stop raw old Set | Same explicit exclusion/preflight required; advisory lock cannot stop old Set |

Recommendation for review: existing-identity versioned native envelope, because moving security-critical fencing out of the native credential protection boundary is the more consequential trust change. Strongest case against it: it is an incompatible representation and preserves a tombstone item instead of physically deleting that item; API-key/raw consumers and presence checks must be adapted. The journal's strongest case is unchanged native payload/removal semantics, but that does not remove its new trust/recovery boundary. Neither representation is implemented or activated. No new reserved credential identity, secret export or automatic migration is proposed. This gate needs representation approval, not reapproval of prompt logout.

Required state-machine semantics whichever representation is selected:

1. Snapshot a non-reusable revision under commit exclusion; never recreate a lost journal/tombstone with a reusable default. Missing/corrupt/unsupported coordination state after adoption fails closed.
2. Before sending a rotating refresh, durably record pending operation under commit exclusion while holding separate refresh exclusion. On restart, unresolved pending means reauthentication (or separately bounded reconciliation), not replay.
3. Logout advances the revision durably and removes secret material under commit exclusion without waiting for issuer/refresh exclusion. Only acknowledge after both fencing and removal succeed. Removal failure leaves the identity fenced, returns failure, and permits explicit idempotent cleanup; issuer calls are not canceled/revoked by this guarantee.
4. Delayed login/refresh must fail final revision validation. Unknown publication outcomes return failure and retain recovery blockers. Newly created Service instances must observe the same state. Cache reads revalidate before returning; tokens already handed to an in-flight remote request cannot be recalled.
5. No compatible-host claim while older raw writers can participate. Explicit adoption/exclusion and startup capability checks remain required; this task does not authorize running adoption. Provider/worker invalidation remains additional unfinished work.

Production coordination implementation is paused at this representation gate; after the required initial in-progress transition, the task was marked blocked using `plan_set_status`. The retained evidence/test is a solved diagnostic unit only; the Phase 3 task and phase are not complete. Config apply/retry remains separately unresolved: failed provider application may retain the prior graph, and identical-baseline reload can skip retry. No live activation is authorized or performed.
