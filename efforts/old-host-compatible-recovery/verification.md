# Phase 1 characterization — 2026-11-09

## Scope and provenance

Characterization only; no production code changed, live configuration/host/VM mutation,
push or merge. Whole-old-host fallback is approved design, not reopened here.
Starting branch `feat/old-host-compatible-recovery`, HEAD
`744fa5e0b7a51a406986615f3c233f15f4c38d16`. The tracing task was already in progress.
Preexisting `.tmp-old-recovery/` is deliberately retained, excluded from checkpoint;
its binary is **not** the binary used for the reproducible test below.

`fixture/run.sh` archives exact repository revision
`d7cbdeb21c44e0ceeefdf212203ed04a37d09150` (server and proto), builds a new
`cmd/oldrecoveryfixture` harness using PATH `go1.26.3 darwin/arm64`, and runs the
current destination regression. `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off` prevents
toolchain/dependency download. No dependency or historical production source
patches are needed by this harness build. It is historical **server implementation
and generated RPC registration**, not a historical shipping CLI executable and
not a current handler returning synthetic Unimplemented.

Adapters: added test main, loopback insecure gRPC, fixture-only authorization,
no-op participation/reload callbacks, retained persistence work, private temporary
store and EOF shutdown. The child receives only disposable HOME/TMPDIR and a
credential-disable setting; it does not instantiate a provider or load user config.
These adaptations bypass production authentication, startup, worker lifecycle and
process attribution. Dirty live manifests cannot certify this as the exact incident
binary. Do not generalize this result to all historical binaries.

## Reproduction and exact results

Commands from repository root unless noted:

* `sh efforts/old-host-compatible-recovery/fixture/run.sh`: PASS,
  `TestActualOldRecoveryCharacterization` selected (not skipped). Old source retains
  work; current destination accepts an operation; old `LifecycleForceRelinquish`
  times out at 150ms with held persistence. Two incomplete journals are written.
  Current `LifecycleResume` returns `Unimplemented: unknown method
  LifecycleTakeoverProbe for service agent.Agent`. Independent candidate discovery
  returns `FailedPrecondition: ambiguous incomplete recovery operations`.
  Durable owner is unchanged and no offer or destination owner is granted.
* `OLD_RECOVERY_ACCEPTANCE=1 sh efforts/old-host-compatible-recovery/fixture/run.sh`:
  expected FAIL, exit 1, at the first missing compatible recovery offer with that
  same Unimplemented error. This is a deliberately opt-in **first red gate**, not
  the completed persisted-turn acceptance journey.
* The normal historical regression then closes its owned child's stdin, waits for
  exit, calls current `RecoverDeadOwner` with a fixture-only proof callback, appends
  a turn and reads it through a separately opened store. This establishes compatible
  historical schema/ownership recovery and persisted write, **not** real process
  attribution, provider execution, or end-to-end coordinator success.
* `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go -C source/server test
  ./internal/conversation -run '^TestRecoveryDoesNotFenceArbitrarySQLConnection$'
  -v -count=1 -timeout=30s`: PASS. A SQLite connection opened before recovery still
  changes conversation content after the destination owns a fresh epoch; the stale
  participating API writer is rejected. Ownership rows/advisory locks are NOT a
  database-wide fence against arbitrary SQL.
* In `source/server`, offline Go:
  `go test ./internal/brewrestart ./cmd/cercano ./internal/conversation -run
  '^(TestUpgradeStopsHostLeavesActiveWorker|TestDeadHostKernelVerification|TestDevelopmentRecoveryRegistryExclusion|TestDeadOwnerRecovery.*)$'
  -count=1 -v -timeout=60s`: PASS. Nonzero selected tests: host-stop/worker-survival,
  kernel death verification, registry exclusion, recovery, stale/reserved/foreign
  child/locked child refusals, atomic failure. These are existing disposable probes,
  not historical shipping-host integration. No broad/race/platform certification.

## Exact cleanup/integration map (no deletions in this phase)

Paths below are relative to `source/`.

| Redundant orchestration and direct consumers | Replacement/retention boundary |
| --- | --- |
| `server/internal/server/resume.go:187-194` calls takeover offer then original recovery; `resume_discovery.go:29,120,189,210` implements mandatory unique journal, offer/confirmation and retry/legacy branches | Single destination coordinator must make unsupported optional RPCs and multiple hints unable to block independently authorized fallback. Keep exact durable state checks; journals never authority. |
| `server/internal/server/takeover_confirmation.go:101-104` directly propagates unsupported old probe; `:151` confirms, prepares, relinquishes, claims. `takeover.go:17,46,49,52,59` implements probe/prepare/receipt/claim/relinquish | Remove parallel takeover product after replacement; preserve exact source epoch, authenticated selected destination, current receipt validation and bounded single-use consent. |
| `server/internal/server/retry_transfer.go:27,45,148`; `server/pkg/agentclient/retry_transfer.go:24,31,38`; `resume_discovery.go:147,265,275,297` | Remove exact-retry RPC orchestration/wrapper, port durability/current-owner/idempotence invariants. |
| `clients/cli/internal/ui/forced_resume.go:30`, `original_recovery.go:14,26,66-90` | Remove parallel original/takeover prompt/routing and response interpretation after single confirmation/result flow exists. Preserve default-No, stale UI-generation rejection, exact pinned connection checks and host-wide disclosure. |
| `proto/agent.proto:23-31`, `:2796-2798`, `:2820`, `:2858`; `server/pkg/proto/agent{,_grpc}.pb.go` | Retry/Takeover RPCs and original recovery fields/messages are removal candidates, not old compatibility controls. Reserve removed field names/numbers (request 14-16, response 15 after consumers migrate); regenerate both bindings. RPC names must not be reused. Do not blindly remove response current_owner (14), selection or confirmation primitives if coordinator reuses them. |

Direct protocol test consumers to port rather than silently delete coverage:
`server/internal/server/{cooperative_takeover,resume_discovery,resume_new_destination,retry_transfer,takeover_pending}_test.go`,
`server/pkg/agentclient/retry_transfer_test.go`,
`clients/cli/internal/ui/{cooperative_takeover,original_recovery}_test.go`,
`server/internal/mcp/server_test.go` (mock/interface surface), plus new characterization.
`resume_test.go` and `coexistence_test.go` retain baseline/old-control safety regressions.

**Retain compatibility controls:** proto `LifecycleResume`, Acquire/Release,
Prepare/ValidateReceipt, Relinquish/ForceRelinquish/Claim, Recover, List/Retire.
Pinned old source contains these older controls but no Retry/Takeover methods.
`server/internal/server/lifecycle_controls.go` holds exact authenticated identity,
loopback control routing, receipt validation and reservation behavior. A mutating
prepare/relinquish call is NOT a safe capability-discovery probe.

**Retain safety below orchestration:** conversation ownership/transaction locks and
write fencing; broker admission/persistence accounting; current claim validation;
publication fsync/durability checks; authenticated source resolution; retained
incoming acceptance; uncertain-outcome blocking. `conversation/dead_owner.go:19`
already performs locked transactional tree recovery, interrupts tasks and preserves
history; `:128` rejects reserved/independent descendants; `:152-158` claims pending
notifications. Preserve distinction between no interrupted-action replay and normal
completion-notification policy. Existing reserved root rejection is an implementation
gap, not license to clear ownership by SQL.

Docs integration: update current flow descriptions in dead-owner recovery and
retry/takeover/original-recovery documentation when replacement lands; historical
verification remains historical. `docs/agent/dead-owner-recovery.md` explicitly
limits same-user launcher proof and detached effects; do not broaden those claims.

## Process, containment and relaunch blockers

Historical `internal/worker/spawn.go:136-144` deliberately does not use
CommandContext. `spawn_unix.go` gives workers independent process groups;
`workerHandle.Kill` can kill its known group, but host death alone cannot do so.
Current `brewrestart/upgrade_darwin.go` treats worker/model-host/mcp-client as
non-host roles. **Passing `TestUpgradeStopsHostLeavesActiveWorker` proves a blocker,
not recovery safety:** worker continues after host stop. The historical harness has
no workers, so its successful EOF teardown supplies no worker-containment proof.

`cmd/cercano/development_legacy.go:34,82,99` wraps private source records, exact
endpoint and generation executable checks; `brewrestart/legacy_darwin.go:15,59`
binds listener, UID, executable identity and host argument before stopping.
`development_recovery.go:17,60,71,84` takes the per-record nonblocking launch lock,
checks retained start intent, rereads identity and verifies absence before returning
held exclusion. `development_launch.go:250-261,470` participates in launch locks.
Pinned old launcher also takes current-target and per-instance flocks (historical
`:219-238`) and records `.starting` (`:284`). Registry exclusion regression proves
lock conflict/refusal and release, not a concurrent actual-old-launcher race.
`VerifyHostAbsent` checks same-user kernel argv, not arbitrary surviving workers.

Remaining safety work: identify/contain exact relevant old workers including detached
or reparented writers; retain exclusion continuously across stop, absence proof and
ownership transaction; test actual relaunch, stale PID/image/epoch/consent and late
reserved claims. Do not infer this from ownership row changes or a missing listener.
Unsupported/uncertain attribution must refuse with a specific blocker. Approved
whole-host stop scope includes sibling interruption and needs explicit disclosure.

## Remaining acceptance and next slice

Phase 1 is **partial**, not completed. Full declined-consent no-mutation assertions,
actual old worker/relaunch journey, and deterministic loopback provider completing
an exact persisted new-host turn remain unimplemented. Current gate fails before
any offer; the independent append above must not be marketed as that acceptance.
Next test slice: extend the fixture with owned worker and launcher participation,
loopback provider and consent/no-consent journey, retaining the current red gate.
Next production slice (later phase only): exact source/worker containment and
continuous exclusion primitive, then one destination coordinator using compatible
old controls and explicit fallback; owned/reserved transactional recovery and
late-claim serialization must precede claiming a ready new host.

## Teardown / retained worktree

Script owns `.old-recovery-fixture.XXXXXX` archive/build directory and trap-removes
it on success/failure. Go `t.TempDir` owns DB/WAL/locks/journals and child HOME/TMPDIR;
EOF plus Wait shuts down historical child, with bounded emergency kill only of the
`exec.Cmd` child. Current fixture listeners/connections/stores close via cleanup.
Native process probes discover/signal only their own helper PIDs and clean them up.
No ambient PID signaling, credentials, provider requests or live state writes.
Preexisting `.tmp-old-recovery/` is retained untracked, not trusted evidence, not
checkpointed or removed. Only explicit tests/fixture/evidence/plan paths are saved.


## Continuation: held source exclusion, not writer containment

Re-read the approved spec/plan and checkpoint `aed3814db2ef`; this slice changes
only the existing development legacy adapter and its disposable registry tests.
`withStoppedSource` retains the existing recovery guard through bounded source
stop, source absence verification, and a callback. Existing `Stop` remains a
source-stop operation; it does not gain a containment claim. The new callback
path is not connected to a production recovery coordinator or ownership mutation.
Cancellation and missing approval validation are rejected before stop. No new
service, recovery protocol, or historical-source adaptation was added.

Fresh bounded check in this continuation (historical fixture pass/red results
above were not rerun in this continuation):

* Same offline environment, `go test -race ./cmd/cercano -run
  '^(TestDevelopmentLegacyProductionAdapter|TestDevelopmentRecoveryRegistryExclusion)$'
  -count=1 -v -timeout=60s`: PASS after final code/test edits.
  Real temporary registry/flocks, stubbed process inspection/stop/absence only.
  Tests cover continuous lock conflict at stop/absence/use, no callback on absence
  failure, callback error propagation, cancellation before stop, lock release,
  stale process/record/generation and rejected approval. Source and unrelated-host
  registry bytes remain unchanged (except intentional stale-record test writes).
  Decline means no Stop invocation; no DB/consent UI journey is claimed. Synthetic
  stale-epoch rejection is the validator seam, not transactional epoch coverage.

### Concrete native blockers and remaining integration

Inspection of `internal/brewrestart/upgrade_process_darwin.go:61-69` shows
`signalIdentity` calls `Inspect`, compares `SameProcess`, then separately calls
`syscall.Kill(id.PID, sig)`. This narrows but does not close the process-exit/PID-reuse
window. It is NOT an atomic identity-bound signal. No new PID signaling code was
added, and these tests never exercise ambient process discovery/signaling.
`upgrade_darwin.go:73-75` explicitly excludes descendants from the host stop;
`VerifyHostAbsent` is not a surviving-worker proof. Fixture-owned child handles
cannot be generalized into attribution for historical detached/reparented workers.
There is consequently no adequate general containment proof in this slice.
Native implementation is darwin+cgo; no other platform or kernel race certification
was performed. Lock tests establish cooperative lock exclusion, not exclusion of
an arbitrary launcher that ignores it or an actual historical relaunch race.

Phase 2 attribution/containment is marked blocked, rather than done. Connecting
this helper to reassignment would require a reviewed identity-bound native stop
and independently justified complete writer attribution/containment. Treating
inspect-then-kill, host absence, or owned fixture handles as that proof would weaken
the approved safety semantics; this continuation stops before such integration.
A future coordinator must also bind disclosed host-wide sibling impact, source,
epoch and selected destination to fresh consent, recheck consent/epoch within the
held callback, and retain exclusion through the ownership transaction. None of
those Phase 3/4 integrations is implemented here. Full Phase 1 owned historical
worker/relaunch, sibling-impact and declined-consent durable-state assertions, and
provider-backed exact persisted-turn acceptance remain incomplete. No production
host/config/VM changes, push or merge; `.tmp-old-recovery` remains untouched and
untracked. No fixture archive/build was created in this continuation.

## Authenticated shutdown continuation (2026-11-09)

The development legacy adapter now uses the already-existing authenticated
`ShutdownAgent`, via private registry credentials and `developmentPeerDial`
(identity plus LifecycleList store binding), then re-inspects the recorded kernel
listener identity before revalidating approval. There is no signal fallback or
new RPC. Accepted is **not** complete: bounded polling of kernel host absence,
then another absence check, runs under the same generation launch lock through
`use`. Refusal, response loss, drain timeout, unreadable processes, stale identity
and bad credentials prohibit continuation. This is integration-ready source stop,
not yet a production coordinator or a writer-containment certificate.

### Corrected relevant-writer boundary

Pinned `d7cbdeb` evidence (historical file lines):
- `cmd/cercano/main.go:1230-1235,2292-2325`: worker entrypoint serves a unix Worker
  service and proxies state to host; it does not open the conversation store.
- `internal/worker/proxies.go:426-515`: main/subagent persistence emits messages
  to the host stream. `internal/worker/host.go:943-975` consumes them and invokes
  host persistence/ensure-child callbacks. A surviving worker cannot execute
  these host callbacks in a dead host.
- `internal/worker/worker_dispatch.go:37-54`: worker tool stack has nil conversation
  store; autonomy is also a host stream proxy, not worker SQLite persistence.
- `internal/worker/tasks.go:25-91`: independent-task controls proxy to the host's
  task service, with parent identity assigned by the host.
- `internal/server/server.go:636-674`: independent tasks use one host-owned
  taskrun manager and the host persistence store. `internal/taskrun/manager.go:
  175,195,210-240` inserts, starts, executes in a goroutine, and commits terminal
  state in that host process. These are not detached standalone store writers.

Thus prior blanket concern that every surviving worker/tool must be killed was
unsupported. Detached tool external effects may remain uncertain; they are not
thereby coexistence writers and must not be replayed. Worker failure/routing logs
and other ancillary stores are not the conversation coexistence store. No native
worker death requirement is introduced based on unrelated effects.

Still missing is runtime qualification of the **selected historical host and its
Worker-service subprocess graph**, against this architecture, including the
production task/dispatch callback route after host shutdown. That concrete class
has not been exercised by the new transport fixture (it uses a real authenticated
gRPC server but injected process checks). The architecture supports a host-only
owned-persistence boundary for this pinned revision; it does not qualify arbitrary
historical executables from a registry record. Actual historical launcher
contention and old worker/task persistence tests are still needed before converting
this source-stop result into coordinator writer authority. There is no evidence
here of an additional detached standalone task SQLite writer or arbitrary SQL
attacker requiring universal descendant containment.

### Executed checks

All commands offline (`GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`):
- Normal command tests: `go test ./cmd/cercano -run
  'Legacy|DevelopmentRecoveryRegistryExclusion' -count=1 -timeout=60s` PASS.
- Race: `go test -race ./cmd/cercano ./internal/brewrestart -run
  'Legacy|DevelopmentRecoveryRegistryExclusion|HostAbsent|RecoveryArgv'
  -count=1 -timeout=90s`: command package PASS; brewrestart explicitly selected
  **zero tests**, not counted as native evidence.
- Corrected native race command: `go test -race ./internal/brewrestart -run
  '^TestDeadHostKernelVerification$' -count=1 -v -timeout=60s` PASS (one selected).
- New `TestLegacyAuthenticatedShutdownWait`: all seven subtests PASS:
  bounded-success, drain-hangs, declined, response-loss, refused, cancelled,
  unreadable-process. These are injected-drain tests, not old in-flight execution.
- New `TestLegacyShutdownAuthenticatedTransport/false` and `/true` PASS: correct
  token shutdown plus held exclusion, wrong token no shutdown/no continuation.
- Existing `TestDevelopmentLegacyProductionAdapter` (12 subtests) and
  `TestDevelopmentRecoveryRegistryExclusion` PASS in the focused normal run.
- `OLD_RECOVERY_PRODUCTION_FIXTURE=1 sh
  efforts/old-host-compatible-recovery/fixture/build-production.sh` PASS: actual
  historical CLI containing host+worker builds offline, with historical
  `developmentfixture cercano_development_acceptance` backend tags, no source
  patch. Build is gated and disposable; **it does not execute the binary**.

No failing executed tests. Full old-production shutdown/in-flight drain journey,
old launcher race and persisted destination turn remain NOT RUN. This build-only
runner is an incremental prerequisite, not the requested runtime acceptance.
No SDK signal probe was necessary: this path uses supported authenticated RPC
and existing native absence inspection, not privilege expansion or PID signaling.

Next coordinator integration slice: add disposable execution of that built
production graph with loopback provider, real worker/task callbacks and old
launcher contention; qualify the pinned writer contract. Then call
`withStoppedSource` with fresh source/epoch/destination disclosure approval and
perform exact owned/reserved recovery inside `use`, retaining exclusion through
transactional late-claim checks. Do not connect Stop alone to reassignment, infer
completion from acceptance, or replay interrupted actions. Plan Phase 1/2 remain
partial; other phases are not marked complete.
