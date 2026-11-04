# Phase 1 bounded baseline verification

## Directly observed — 2026-11-03

- Read Phase 1 of `plan.md` and `docs/agent/self-dev.md`. The effort directory contained only `plan.md` and `spec.md`; no previous verification file or saved baseline artifacts were available. No broader audit was performed.
- Read all Go tests and implementation in `source/server/pkg/update` before execution. Tests use temporary cache/install fixtures and local HTTP servers (plus localhost:1 for failure). Successful release checks call `DetectInstallMethod`, which may execute read-only `brew list cercano`: this suite is not fully hermetic, but contains no package installation, live binary replacement or agent launch actions.
- Ran exactly one focused test command, from `source/server`:

  ```text
  go test ./pkg/update -count=1 -timeout=60s
  ok  cercano/source/server/pkg/update  1.297s
  exit 0
  ```

- This directly establishes the update package's existing unit-test baseline, including advisory upgrade guidance and unchanged temporary fixture binaries/symlinks. It does not establish survival of native-manager upgrades, package cleanup, installed host/session continuity or automatic retirement. No failing probe was observed in this run.
- No separate builds, installs, live-agent actions or manual git inspection were performed. Source and specification were not changed.

## Prior reported trace — not independently revalidated here

The recovery task reports that installed executable/resource paths remain replaceable. Development-generation retention is a reuse candidate, not proof that installed discovery, reconnect and later worker starts use complete retained bundles after native-manager replacement or cleanup. These are reported baseline findings, not results of the test above; no saved detailed trace was available in this effort directory to substantiate additional path/line claims.

Reuse boundary: preserve native-manager package ownership; consider development-generation retention and session binding only as narrowly tested implementation candidates. Do not import the old custom-updater branch wholesale. Installed resource completeness, shared-runtime nonreplacement, shared-store admission and safe retirement still need their own evidence.

## Acceptance blockers and remaining scope

Disposable native runners have not been provided/established for Homebrew/macOS, Debian/APT Linux, or Chocolatey/Windows. Missing resources are disposable machines/VMs, fixture native package sources/artifacts and isolated user/session state; Windows locked-file and Unicode/space-path coverage also requires a native Windows runner. No runner provisioning or credential search was attempted. This is an installed-system acceptance blocker, not a platform pass; production signing/publication remains separately unauthorized.

The bounded recovery is complete, but Phase 1 as a whole is not. Installed-discovery, pinned-session, worker-generation, modelhost nonreplacement and formula-rendering baselines were not rerun. Trace and broader baseline tasks remain in progress. Only the disposable-native-environment task is changed to blocked; no existing completion or branch/base identity claim is newly verified here.

## Local continuation — 2026-11-03

### Authorization clarification

The current user explicitly authorizes continuing local implementation with native acceptance deferred. This bounded turn is **baseline documentation only**, not source implementation. Native installed-system acceptance remains mandatory before support claims, and the native-environment task remains blocked. No live install/config/agent mutation, model startup, installation build, credential provisioning, publication, push or merge was performed.

Read AGENTS.md, docs/agent/self-dev.md and the effort plan/spec/verification. The following is fresh source inspection, not native execution evidence.

### Installed path and reuse boundaries

- `source/server/pkg/agentclient/client.go:95-125,187-201,223-266`: ordinary Dial first reuses a reachable server, otherwise locks/rechecks, discovers a sibling `cercano` then PATH, and starts `cercano agent`. There is no retained-bundle selection in that discovery. `reconnect.go:347-366` redials and can call ensureServerLaunched again; ordinary reconnect is not generation pinning.
- `source/server/cmd/cercano/main.go:1187,1299` chooses legacyProductionStartup for ordinary entry; `production_startup.go:75-91` runs coexistence preflight only for nonlegacy admission. `main.go:355-386` constructs a generation SpawnPolicy only with a generation binding. `ordinary_generation_host.go:18-30` validates binding/activation before loading config: useful integration boundary, not proof of installed admission.
- `source/server/internal/worker/spawn.go:106-109,210-222,373-385`: later worker startup resolves through policy and invokes `cercano worker`; unpinned lookup consults the current executable sibling then PATH. `policy.go:60-106,145-154` pins generation binary and manifest contents for explicit generation policy, while zero policy uses legacy lookup. Package replacement/removal can therefore redirect or break later legacy lookup; that is a source-derived risk, not a reproduced failing test here.
- `source/server/pkg/agentclient/pinned_session.go:49-83,113-124`: explicit pinned-session dialing/reconnect validates host, contract/schema and retained session rather than launching a replacement. Reuse that admission boundary, not ordinary reconnection as evidence of continuity.
- Known bundle inputs: host/worker `cercano`, terminal `cercano-cli`, and generation manifest. `source/server/scripts/publish-dev-generation.py:282-288` accepts exactly those three members; this does not prove completeness of external runtimes. `internal/localruntime/llamaserver/provider.go:824` resolves `llama-server` via PATH; `internal/localruntime/mistralrs/provider.go:450` similarly resolves `mistralrs`. External runtime executables, their libraries/assets, model files and configuration authorities need explicit retention/admission accounting. No exhaustive platform dynamic-library/resource inventory was established in this bounded pass; the trace task stays in progress.

### Package commands and remote-only CLI

- `release/homebrew/cercano.rb.in:17-25` currently gives one formula both bin/cercano and bin/cercano-cli and invokes restart-after-upgrade from post_install. Lines 30-35 advertise restart. This is existing behavior to replace, not approved upgrade policy or evidence of split package readiness.
- `source/clients/cli/main.go:124-131` constructs localhost plus configured port and calls auto-launching Dial. The ordinary path inspected does not expose a remote host selector. `agentclient/client.go:128` offers DialExisting as a no-launch primitive, but that alone does not establish standalone remote CLI support. The intended dependency-only convenience package and ownership of the everyday cercano command remain implementation work; do not infer executable renaming from package names.

### Runtime replacement and shared data

- `source/server/internal/modelhost/ensure.go:193-212,249-268` retains a default older-host shutdown/replacement path; RefuseOlderHost and CoexistenceProtection gate it. `cmd/cercano/main.go:955` sets RefuseOlderHost for ordinary startup; `development_host_production.go:70-72` requires coexistence protection. These are different guarantees: refusing an older host is not proof of session-lifetime activation deferral or complete caller coverage.
- `production_startup.go:86-105,154-186` separates preflight/validated generation activation and exact borrowed prepared store from legacy access. `internal/conversation/store.go:392-397` documents fail-closed handling before persistent pragmas/migrations; migration calls still exist at 453-486. Shared config is loaded at `ordinary_generation_host.go:26` and persisted/watched at `main.go:591,621,688`. No proof was obtained that every admitted installed generation is protected against incompatible configuration/schema changes. Runtime-caller and admission trace remains in progress rather than claiming an exhaustive audit.
- Reuse only explicit generation validation, pinned reconnect, worker content validation and protected runtime attachment where tests establish their boundaries. Do not import the old custom updater wholesale, or carry forward acquisition, installation, verification/staging, rollback or mandatory restart machinery. Native managers remain delivery owners; development publication is not a portable production installer.

### Exact fresh test results

All commands had explicit tool timeouts. The selected Go fixture isolation properties were reported in prior context, not independently revalidated in this turn; because Go could not execute, no Go fixture or model process started. The Python rendering suite ran without package installation. No real model launch was requested.

1. From source/server, tool timeout 55s:
   `go test ./pkg/agentclient -run '^(TestBinaryDiscoveryInstalledLayout|TestInstalledAgentReuse|TestPinnedNoLaunchRefusals|TestPinnedTransportAndFreshValidation|TestPinnedTerminalIdentityLifetime)$' -count=1 -timeout=35s`
   Tool failed before test execution: `fork/exec /opt/homebrew/bin/go: no such file or directory`. No test pass/failure or exit code was reported by the runner; this is an environment blocker.
2. From repository root, tool timeout 20s:
   `python3 -B -m unittest discover -s release/homebrew -p test_render_formula.py`
   Exit 0; `Ran 11 tests in 0.012s`, `OK`. Rendering checks only; no native package operations. Passing current rendering tests does not endorse the restart hook.
3. From source/server, tool timeout 45s:
   `go test ./internal/worker ./internal/modelhost -run '^(TestDevHostWorkerGenerationReplacementRejected|TestEnsureRefuseOlderHostAtBothProbes|TestEnsureRefuseOlderHostStillLaunchesAbsentHost)$' -count=1 -timeout=30s`
   Same pre-execution missing-Go error. No tests executed. No toolchain installation/search outside the worktree was attempted.

The earlier pkg/update result above remains historical, not rerun. The local baseline is useful but **not complete**: Go execution, exhaustive resource inventory and remaining caller/configuration audit are unresolved. No new failing behavior was reproduced; no source fix or test fixture was added.

### First Phase 2 failing-fixture recommendation

Add an isolated installed-flow test adjacent to installed_reuse_test.go and worker generation tests: launch a copied test helper from a temporary manager-style A bundle, admit an A session through the actual installed selection boundary, then replace prefix selection with B and remove only fixture-owned A package files. Request a later A worker through the real worker selection path, recording a generation marker and requiring an A-only resource read. Assert the worker still uses retained A executable/resources, never B or hostile PATH; separately assert a new eligible session can use B. Start with the later-worker-after-A-removal assertion as the smallest red test. Current unpinned worker lookup is expected to fail or pick B; this expectation is not a run result. Use fake runtime/RPC endpoints, isolated HOME/TMPDIR/config and test-owned processes only. Extend to missing libraries/assets and interrupted retention after the first failure is captured. Do not use the live Cellar or launch models.

## Local Phase 1 closeout — 2026-11-03

This section supersedes the earlier missing-Go execution blocker and incomplete caller-audit notes, not the native acceptance blocker. AGENTS.md, plan.md and verification.md were read. No source implementation, package rendering rerun, Git plumbing, native-manager operation or live configuration/runtime mutation was performed.

### Focused baseline execution

Executed via PATH from `source/server`, each subprocess bounded to 55 seconds (outer tool 115 seconds):

```text
go test ./pkg/agentclient -run '^(TestBinaryDiscoveryInstalledLayout|TestInstalledAgentReuse|TestPinnedNoLaunchRefusals|TestPinnedTransportAndFreshValidation|TestPinnedTerminalIdentityLifetime)$' -count=1 -timeout=35s
ok cercano/source/server/pkg/agentclient 5.780s
exit 0
go test ./internal/worker ./internal/modelhost -run '^(TestDevHostWorkerGenerationReplacementRejected|TestEnsureRefuseOlderHostAtBothProbes|TestEnsureRefuseOlderHostStillLaunchesAbsentHost)$' -count=1 -timeout=30s
ok cercano/source/server/internal/worker 0.914s
ok cercano/source/server/internal/modelhost 0.578s
exit 0
```

A Python TemporaryDirectory confined HOME, TMPDIR, XDG_CONFIG_HOME, XDG_CACHE_HOME and CERCANO_SEARCH_SCRIPT_DIR. GOPATH and GOCACHE pointed at the existing user's Go module/build caches; this is fixture-state isolation, not a hermetic toolchain build. The temporary state was removed on completion. Earlier successful update and 11 rendering tests remain historical evidence, not rerun results. No baseline failure was observed; the package-removal defect below remains a source-derived hypothesis until Phase 2 adds its red fixture. These focused passes do not establish installed upgrade continuity.

### Implementation dependency inventory

| Dependency / ownership | Source evidence and Phase 2 treatment |
| --- | --- |
| Agent/worker executable | `internal/worker/policy.go:60-106,145-160` selects `cercano` inside an immutable generation and hashes executable/manifest; zero policy falls back to installed lookup. Retain the executable independently of manager files before admitting a session; do not mistake digest checking for retention. |
| CLI executable and commands | `release/homebrew/cercano.rb.in:17-25` installs both executables in one formula and invokes restart-after-upgrade; `source/clients/cli/main.go:124-131` uses localhost and auto-launching Dial. CLI remains a distinct package-owned executable; standalone remote endpoint/no-launch admission needs implementation, not renaming the agent. |
| Manifest and worker state | `policy.go:60-106` requires manifest, canonical generation path, short existing private ResourceRoot and unique host namespace. Inventory hashes, platform/architecture and resource paths; runtime socket/log state is mutable host-owned state, not copied package data. |
| Embedded resources | `go:embed` sites: conversation/schema.sql, skills/catalog, agent/prototypes.yaml, contextmeter/cl100k_base.tiktoken, localruntime/{llamaserver,mistralrs}/catalog.json, pkg/config/tier_recommendations.yaml, web/ddg_search.py. These travel with the executable; do not copy repository-relative substitutes. |
| Materialized web helper | `internal/web/script.go:17-75` writes an embedded script to shared home scripts unless explicitly redirected; `web/search.go:44` executes explicit Python/script paths. `ddg_search.py:15` requires ddgs; `pkg/config/config.go:1300-1309` locates a user venv. Use generation-private materialization (existing CERCANO_SEARCH_SCRIPT_DIR seam); shared refresh can otherwise replace A's later script. Python/venv dependencies require explicit compatibility or retained ownership, not an assumption that two Go binaries suffice. |
| Model runtime and assets | llama-server and mistralrs are PATH-discovered external executables (`localruntime/llamaserver/provider.go:824`, `mistralrs/provider.go:450`); configured model directories and optional mmproj are external assets. Models/config/credentials/conversations remain user authorities, not package cleanup targets. Account for external runtime executable/library/backend/model compatibility through runtime admission; do not silently copy models or invoke live providers in the fixture. |
| Platform libraries | `scripts/build-macos-release.sh:59-69` enables CGO, while Linux/Windows release scripts disable it. Release archive lists include two executables, LICENSE and README, not external runtimes. Build flags do not prove a complete dylib/DLL/backend closure. No release artifact loader inspection was performed; native artifact inspection must enumerate non-system dependencies and distinguish OS-provided dependencies from retained files. |

The source-level inventory is sufficient to start the executable/resource fixture, **not an exhaustive per-platform dependency closure**. The discovery/dependency task remains in progress for that explicit unresolved portion; native acceptance and complete bundle eligibility cannot be asserted from these tests.

### Remaining replacement and admission caller audit

- Production `modelhost.NewRemote` callers found in `cmd/cercano/main.go:953`, `development_host_production.go:70`, and `dev_host.go:221`; `internal/modelhost/remote.go:113-124` funnels them through Ensure and propagates protection on reconnect. Ordinary startup requests RefuseOlderHost; prepared/fixture hosts request CoexistenceProtection. `ensure.go:193-212,249-268` retains default replacement, but these call-site policies gate it. Refusal of an old host is not session-lifetime compatibility tracking.
- Explicit worker restart flows `worker/runtime_control.go:34` → `worker/host.go:1009-1025` callback → `server/server.go:5146-5153` → `internal/runtimecontrol/restart.go` → public RestartRuntime (`server.go:3257-3275`) → manager. `modelhost/server.go:857-860` explicitly restarts for all holders unless coexistence-protected; protected shutdown/StopAll checks are at 390-397 and 213-215. Attachment holds and release cleanup (475-526) are not proof of full idle-session retention. This is a Phase 3 deferral seam, not grounds to disable explicit user controls wholesale.
- The package hook still invokes the separate forced-restart machinery. `internal/brewrestart/coordinator_darwin.go:107-122` requests ShutdownAgent; `upgrade_darwin.go:16-19` and legacy helpers can terminate verified hosts. These existing paths must not become the native upgrade safety policy; package hook removal belongs to Phase 4. None were executed.
- `production_startup.go:75-105` preflights nonlegacy conversation compatibility and requires the exact prepared store; prepared construction validates admission before config load (`development_host_production.go:83-97`), as does ordinary generation activation (`ordinary_generation_host.go:18-26`). Legacy `conversation.Open` refuses marked stores before persistent pragmas and rechecks under a write transaction (`conversation/store.go:392-450`), but unmarked legacy stores still initialize/migrate (453 onward). Installed activation needs to enter the generation admission path, not silently adopt the legacy opener.
- Correction to older comments/audit wording: current `pkg/config/config.go:993-1078` captures a snapshot and performs legacy normalization in memory; it does **not** itself save migrated YAML. `SaveSnapshot`/`Save` at 1327-1340 use the coordinator; `coordination.go:173-214` rejects read-only state and stale revisions before atomic publication. This protects against lost updates, not all cross-generation semantic incompatibility. Whole-document save and live config watchers still require an admitted-generation compatibility contract. Do not claim existing snapshot coordination proves session-safe schema activation.

Caller/admission tracing is complete for the identified local production paths and records the unsafe seams; native behavior, session-lifetime deferral and cross-generation configuration safety remain unimplemented/unverified, not audit passes for those features.

### Concrete next Phase 2 unit (no implementation in this checkpoint)

1. Add `source/server/internal/worker/installed_retention_test.go`, reusing helper-process patterns from `pkg/agentclient/installed_reuse_test.go` and digest cases from `internal/worker/generation_replacement_test.go`. First red assertion: after selecting A, unlink fixture-owned A package files and switch a fake manager prefix to B; a later A worker must execute A and read an A-only resource, never B or hostile PATH. Exercise `SpawnPolicy.workerBinary` and `workerCommandWithPolicy` rather than testing a new copy helper alone. A copied test executable handles a worker-mode marker without launching a real model.
2. Extend `pkg/agentclient/installed_reuse_test.go` at the actual `findCercanoBinary`/launch seam with that manager-layout helper: reachable retained A stays A, a later eligible new session selects B, pinned reconnect never falls back to ordinary auto-launch. Keep an explicit remote/no-launch case independent of local agent installation. No such fixture has yet been added or run.
3. Implementation touchpoints after capturing red: narrow installed-generation retention/validation support; `pkg/agentclient/client.go` installed selection; `cmd/cercano/production_startup.go` and `ordinary_generation_host.go` admission; `internal/worker/policy.go` plus spawn wiring for immutable executable/resource authority. Reuse canonical path/hash/private-resource checks, but add complete manifest membership and atomic publication before eligibility. Do not import the development publisher as a package installer.
4. Required fixture inventory: two copied helper executables, distinct A/B resource bytes and manifests, fake manager-owned roots/prefix link, independent retained root, short private worker-resource root, fake RPC/modelhost endpoints, isolated home/config/script directory, and explicit test-owned process cleanup. Missing manifest/resource and interrupted publish must leave prior A selection intact. Library closure is a later native-artifact case, not simulated proof of dylib/DLL correctness.

Native Homebrew/APT/Chocolatey install/upgrade/cleanup, Windows locks and Unicode paths, signing/publication, and exhaustive shipped dependency closure remain deferred. No platform support or automatic safe retirement is claimed.

## First Phase 2 test-only slice — 2026-11-03

Added `source/server/internal/worker/installed_retention_test.go`. The first Phase 2 task is in progress, not complete: this is a worker-policy characterization, not installed host/session admission or native upgrade acceptance. No production retention code was added.

### Observed red probe, then passing characterization

Executed through Go on PATH from `source/server`, with 60-second tool bounds and Go `-timeout=40s`:

1. `go test ./internal/worker -run '^TestInstalledRetentionCharacterization$' -count=1 -v -timeout=40s` initially **failed** (exit 1, package 1.367s). With the desired final assertion that A must still run, the actual result was `worker A: worker policy: lstat <TempDir>/manager/A: no such file or directory`, output empty. No production fix preceded this probe.
2. Replaced that desired-success assertion with explicit missing-dependency expectations, preserving suite health without claiming acceptance. `go test ./internal/worker -run '^(TestInstalledRetentionCharacterization|TestDevHostWorkerGenerationReplacementRejected|TestPinnedPolicySelectionAndNamespaces)$' -count=1 -v -timeout=40s` **passed** (package 1.382s).
3. `go test ./internal/worker -run '^TestInstalledRetentionCharacterization$' -race -count=1 -timeout=40s` **passed** (package 7.107s). Full suite not run. An initial tool call with relative cwd failed before execution (`/bin/bash: no such file or directory`); absolute worktree cwd resolved it.

Actual marker sequence: A/A asset, B/B asset, A/A asset after alias replacement; removing only A's resource still passed `workerBinary()` validation but the child exited 93 with `open <TempDir>/manager/A/A-only.resource: no such file or directory`. Removing the remaining A package caused policy validation to refuse before launching a later A worker. B continued to execute B/B asset. Thus pinning prevents redirection to B but does not retain A, and binary/manifest validation does not validate asset closure.

The fixture resolves a fake manager alias and admits policies with `NewSpawnPolicy`, then runs copied test executables via `workerBinary` and `workerCommandWithPolicy`. This is deliberately below installed discovery/session admission: there is no RPC/session acceptance claim. Helpers check sibling generation markers and generation-specific resources and exit without agents/models. All helper binaries/resources and package deletion are in `t.TempDir`; only short private socket namespace allocation uses the existing `shortPinnedFixtureRoot` seam. Children receive explicit private HOME/TMPDIR and fixture-only PATH and have a ten-second kill bound; no live config, credentials, packages, agents, models or stores are accessed by the helper. No fake retained copy is constructed.

### Minimal viable next implementation seam

Introduce narrow installed-runtime publication before host/session admission: resolve the selected manager package, inventory and validate required executable/resource members, copy into an independently owned staging generation, validate copied bytes and completeness, then atomically publish an immutable eligible bundle. Pass that canonical retained path into existing `SpawnOptions` / `NewSpawnPolicy`; later A worker launches must derive both executable and asset authority from A's retained bundle, not the manager alias or PATH. Keep mutable worker socket/log ResourceRoot separate from retained package resources. Interrupted or incomplete publication must not replace prior eligibility. This is retention, not package acquisition/installation or rollback.

Existing development publication is not sufficient: `source/server/scripts/publish-dev-generation.py:282-288` permits exactly manifest.json plus cercano and cercano-cli, hashing the binary entries. Worker policy validates and pins only its executable and manifest. A two-binary manifest cannot establish the A-only resource (or library/backend) closure shown missing here. Extend explicit member inventory/digests and safe relative-path validation for required assets before integrating installed discovery/admission; do not merely copy two binaries and call the bundle complete. Platform dynamic-library closure still needs native artifact inspection.

Next acceptance assertions should replace this characterization only when real runtime retention exists; extend through installed discovery, host/session binding, reconnect and interrupted publication. Native Homebrew/APT/Chocolatey acceptance, Windows locked/Unicode paths and automatic retirement remain deferred. No security or shared-data policy changed.

## Bounded Phase 2 retention primitive + worker fixture — 2026-11-03

Implemented `internal/runtimebundle` as an additive, caller-inventoried copy and
validation primitive, not package delivery or installed launch integration.
Required members carry safe slash-relative paths, expected SHA-256 bytes and an
executable flag. Fresh private slots use `.staging`, validate all required copied
members, then atomically rename to `bundle`. No current pointer or existing
bundle is modified. No automatic cleanup occurs, including failed stages.
`os.Root` confines source opens; symlink components/nonregular members, traversal,
duplicate paths and overlapping source/retention roots are refused. The existing
Python publisher and worker-private safety code informed stage/validate/rename
and file/digest checks; they could not directly implement the additive asset
inventory without changing their contracts. No existing manifest changed.

`TestInstalledRetainedBundleWorker` exercises real `Publish` followed by the
existing `NewSpawnPolicy`, `workerBinary` and `workerCommandWithPolicy` seams.
Retained A executes A/A-only asset after the fake manager switches to B, after
A's source asset disappears, and after removal of manager A. B still executes
B/B-only asset. Republishing with A's required asset missing fails. The original
`TestInstalledRetentionCharacterization` remains a separate nonretained failure
case. All copies/removals remain inside temporary fixtures; no live install,
shared data, package manager, remote CLI, permissions policy or security policy
was modified. No helper starts an agent/model.

### Commands and observed outcomes

Executed in `source/server` through Go on PATH on Darwin, Go timeout 60 seconds
and tool bound 90 seconds:

- Initial combined focused run: worker characterization and retained success both
  passed; package preservation assertions failed because macOS `/var` TempDir
  aliases were compared against canonical `/private/var` publication paths.
  Canonicalized the package fixture roots; no production relaxation was needed.
- `go test ./internal/runtimebundle -count=1 -v -timeout=60s` — PASS (0.432s).
  Covers missing/tampered files, pre-cancellation and deterministic mid-copy
  cancellation with abandoned ineligible staging, symlink files/parents,
  nonregular members, unsafe/duplicate paths, overlapping roots, unlisted-file
  exclusion, executable/private modes, retained tampering and four concurrent
  independent publications surviving source deletion.
- `go test ./internal/worker -run '^(TestInstalledRetentionCharacterization|TestInstalledRetainedBundleWorker|TestDevHostWorkerGenerationReplacementRejected|TestPinnedPolicySelectionAndNamespaces)$' -count=1 -v -timeout=60s`
  — PASS (2.722s); retained helper marker sequence A, B, A, A, A, B.
- `go test ./internal/runtimebundle -race -count=1 -timeout=60s` — PASS (1.437s).
- `go test ./internal/worker -race -run '^(TestInstalledRetentionCharacterization|TestInstalledRetainedBundleWorker|TestDevHostWorkerGenerationReplacementRejected|TestPinnedPolicySelectionAndNamespaces)$' -count=1 -timeout=60s`
  — PASS (15.379s).

### Honest completion boundary

This bounded primitive/fixture unit is complete. The broad Phase 2 retention and
interrupted-publication tasks remain **in progress**, not done: production
admission/selection is unconnected. No installed session/reconnect or native
upgrade acceptance is claimed. Whole repository tests, hard-process-kill and
power-loss recovery, disk/permission fault injection, native Windows locks/ACLs,
and Homebrew/APT/Chocolatey acceptance were not run.

Inventory contract and limitations are recorded in
`source/server/internal/runtimebundle/README.md`. An asset omitted from the
caller-provided inventory cannot be detected; a required inventoried asset
missing from the source fails. Existing manifests are opaque bytes. There is no
persisted inventory/recovery schema, symlink/metadata preservation, loader
relocation or full dynamic-library closure. Worker production validation remains
binary/manifest-only; the fixture succeeds by retaining its explicitly known
resource, not by connecting installed discovery or solving general resource
lookup. Private root ownership remains a caller precondition, not a new security
policy or native ACL claim.

Exact remaining seam: resolve the installed selection in
`pkg/agentclient/client.go`, produce a trustworthy complete platform inventory,
call retention before `cmd/cercano` host/session compatibility admission, and pass
the returned canonical generation and manifest to `SpawnOptions`. Bind production
resource lookup, reconnects and later workers to that same authority, retain
inventory for recovery/revalidation, then prove native package cleanup behavior.
No cleanup, package database, downloads, rollback or shared-data migration is
included. Native acceptance remains deferred.


## 2026-11-03 — Build-produced release payload inventory

Explicit user clarification: llama.cpp is a version-locked, tested internal
agent-host dependency delivered with agent native packages, NOT an arbitrary
external PATH dependency. This records the new approval without rewriting the
approved spec. Production runtime lookup has not changed in this slice.

Added schema-1 `runtime-inventory.json`: release version, platform identity,
lexically sorted safe relative member paths, SHA-256 and executable booleans.
The inventory excludes itself to avoid recursive hashing. Shared Python production
and canonical validation use the runtimebundle Member semantics; that Go helper
has no reusable on-disk schema. Embedded inventory was chosen over a detached
sidecar to keep bytes and inventory in the same payload. Exact archive layout
gates remain authoritative and now require the inventory (older inventory-free
archives fail the updated gates). Decision ledger capture was attempted but the
tool reported no active autonomous run; this paragraph records the decision.

Linux/Windows generate at their final staging boundary, then verify archived
bytes and inventory before promoting the archive. Signed macOS generates only
AFTER staged codesign verification; the unsigned rehearsal does likewise after
staging. Homebrew archive validation and macOS consumer verification require
the same inventory contract. Fixture signing changes bytes and tests assert the
inventory hashes those changed bytes, not the pre-signing input.

Scope is ONLY actually packaged bytes: the two Cercano binaries, LICENSE and
README. This is not complete dependency closure, authenticity proof, retained
runtime admission, native package installation, or evidence of a llama payload.
No llama version or payload has been invented. Inspection of `scripts/`,
`release/`, `.github/` and `source/server/Makefile` with recursive llama/cmake
search found only llama provider-test references in test-release-workflow.py
and release-macos.yml, no llama version lock or build recipe. Builders still
build only the two Go binaries. Needed next: an exact llama.cpp source revision
and reproducible per-platform build recipe with dependency/library inventory,
plus compatibility qualification evidence on the supported platforms before
that revision can be called tested and shipped. No upstream pin selected.

Fresh focused verification (all passed):
- `python3 -B scripts/test-runtime-inventory.py`: 3 tests; deterministic output,
  unsafe paths, changed bytes/modes, identity mismatch, missing/extra/duplicate
  members, absent inventory, overwrite and symlink refusal across tar/ZIP fixtures.
- `python3 -B scripts/test-linux-release-build.py`: 14 tests.
- `python3 -B scripts/test-windows-release-build.py`: 11 tests.
- `python3 -B scripts/test-macos-release-build.py`: 12 tests, including unsigned
  staging and post-signing byte hashes.
- `python3 -B release/homebrew/test_render_formula.py`: 11 tests.
- `python3 -B scripts/test-macos-release-verify.py`: 23 tests.
- `python3 -B scripts/test-release-workflow.py`: 47 tests.
- `bash -n scripts/build-macos-release.sh scripts/build-macos-unsigned.sh`: passed.

Failure probes preceded fixes: Linux sandbox launcher lacked the imported
inventory module on sys.path (runpy does not add the script directory); fixture
launchers now explicitly add it. Windows exact-layout expectation initially
omitted the newly packaged inventory; updated after reproducing that failure.
These are stubbed/inert archive tests, not real compiler/signing or installed
acceptance. No downloads, publication, live install, credentials or model startup
were used; developer installation is unchanged. No production runtime lookup
or Go runtimebundle behavior changed. Native installed-system acceptance,
actual Developer ID/notarization, loader/library closure, Windows ACL/locking,
and llama qualification remain unverified/deferred.
