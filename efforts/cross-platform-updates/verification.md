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


## Configured llama baseline lock and read-only Darwin closure (2026-11-03)

Implemented `release/llama/baseline.lock.json`, its JSON schema, offline
`scripts/llama_baseline.py` and fixture tests. The release workflow's existing
test gate validates metadata before credential import. No archive layout,
installer, runtime lookup, or runtime inventory contract changed; archives still
do not bundle llama. The baseline is not native acceptance or build provenance
for a future Cercano artifact. Source revision and observed bottle provenance
are separate, with unknown upstream tag/source digest/artifact digest/recipe
explicitly null and no qualified platforms. The schema deliberately cannot
assert bundled delivery. See `release/llama/README.md` for evolution rules.

Evidence: parent supplied configured binary `/opt/homebrew/bin/llama-server`,
symlink `../Cellar/llama.cpp/9890/bin/llama-server`, `--version` 9890
(74976e1ae), AppleClang 21 / Darwin arm64, and no observed running llama process.
This slice did not reread configuration or start llama. Read-only inspection used
`otool -L`, `otool -l`, `codesign -dv --verbose=2`, directory/symlink enumeration,
and selected INSTALL_RECEIPT.json fields. No binary copies, model startup,
network, signing operations or live changes occurred.

The llama receipt independently confirms source revision
`74976e1aefd8d64845ae5510674b36079c855c0c`, stable package 9890, arm64,
built/poured bottle and compiler clang. Declared runtime dependencies are ggml
0.15.3, libomp 22.1.8, openssl@3 3.6.3, ca-certificates 2026-05-14; all revisions
0 and rebuilds 0 except OpenSSL rebuild 1. These are *historical receipt
requirements*, not a pin on currently resolved bytes. Inspected opt links resolve
ggml 0.15.3, libomp 22.1.8, **OpenSSL 3.6.5**, and **CA certificates
2026-09-25**. Current OpenSSL's receipt declares the latter. Thus a local version
string or original receipt alone would incorrectly describe current closure.

### Static loader closure and dynamically loaded backend candidates

Below, G = `/opt/homebrew/opt/ggml/lib/`, O =
`/opt/homebrew/opt/openssl@3/lib/`, L = llama.cpp 9890 `lib/`.
Dylib install IDs (the first line for dylibs in `otool -L`) are not dependency
edges. Every listed non-system target exists; versioned llama/ggml aliases
resolve to the real files shown. The initial shallow walk marked four @rpath
edges unresolved; explicit LC_RPATH expansion and directory inspection resolved
them in L, correcting that limitation.

| Node | Non-system load edges (excluding own install ID) |
|---|---|
| `bin/llama-server` | `@rpath/libllama-server-impl.dylib`, `@rpath/libllama-common.0.dylib`, `@rpath/libmtmd.0.dylib`, `@rpath/libllama.0.dylib`, G `libggml.0.dylib` and `libggml-base.0.dylib`, O `libssl.3.dylib` and `libcrypto.3.dylib` |
| L `libllama-server-impl.dylib` | same llama/mtmd/ggml/OpenSSL edges as server, excluding itself |
| L `libllama-common.0.0.9890.dylib` | `@rpath/libllama.0.dylib`, G ggml/base, O ssl/crypto |
| L `libmtmd.0.0.9890.dylib` | `@rpath/libllama.0.dylib`, G ggml/base |
| L `libllama.0.0.9890.dylib` | G ggml/base |
| ggml `libggml.0.15.3.dylib` | `@rpath/libggml-base.0.dylib` |
| ggml `libggml-base.0.15.3.dylib` | `/opt/homebrew/opt/libomp/lib/libomp.dylib` |
| libomp 22.1.8 `libomp.dylib` | none |
| OpenSSL 3.6.5 `libssl.3.dylib` | absolute `/opt/homebrew/Cellar/openssl@3/3.6.5/lib/libcrypto.3.dylib` |
| OpenSSL 3.6.5 `libcrypto.3.dylib` | none |

This is 10 distinct Mach-O files including the executable. Server and llama/ggml
libraries use LC_RPATH `@loader_path/../lib`; libomp and ssl/crypto have no
LC_RPATH. Dylib install IDs are absolute Homebrew opt paths. System load edges
in this graph are `/usr/lib/libSystem.B.dylib`, `/usr/lib/libc++.1.dylib`, plus
CoreFoundation and Security frameworks from llama-common/server-impl; these are
OS dependencies, not files to copy into a runtime bundle.

Static closure is insufficient: ggml `libexec/` also contains five arm64 backend
modules. All have LC_RPATH `@loader_path/../lib`, depend on
`@rpath/libggml-base.0.dylib`, libc++ and libSystem:

- `libggml-blas.so`: additionally Accelerate framework.
- `libggml-cpu-apple_m1.so`, `libggml-cpu-apple_m2_m3.so`,
  `libggml-cpu-apple_m4.so`: additionally Accelerate and absolute opt libomp.
- `libggml-metal.so`: additionally Foundation, Metal, MetalKit,
  CoreFoundation frameworks and `/usr/lib/libobjc.A.dylib`.

These candidates make 15 files if all five backends are required. This is not a
claim that all are loaded or sufficient: backend discovery paths, runtime CPU
selection, embedded Metal resource behavior and OpenSSL provider/config/trust
resource requirements remain unqualified. No loose `.metal` or `.metallib` files
were found under the inspected llama and ggml Cellar roots. Headers and CMake
metadata do not establish runtime resource completeness. Unrelated llama CLI,
bench, completion, quantize, perplexity and fit-params implementation libraries
exist but are not static dependencies of the server graph above.

### Relocation/signing blockers and next steps

All inspected server-closure libraries and backend modules report ad hoc
signatures with no TeamIdentifier; BLAS/Metal modules are linker-signed ad hoc.
This is not Developer ID/notarization or signature-verification evidence.
Absolute opt load edges and the versioned Cellar ssl-to-crypto edge escape any
retained private bundle. Copying binaries, even with their @rpath siblings,
would still depend on mutable manager-owned paths. Backend discovery and TLS
trust/provider resources can escape the static graph too. A source revision
lock does not fix these issues.

Next concrete locally implementable slice: add an offline Darwin loader-report
parser/closure validator with text fixtures for the graph above, @rpath and
@loader_path expansion, symlink aliases, opt-version drift, backend candidates,
missing resources and escaping absolute paths. Define the proposed private
bin/lib/libexec layout and reject external non-system edges; do not wire live
lookup or claim relocation from fixtures. After that, implement an explicit
source/dependency build recipe (no live bottle copying), resolve backend/resource
and license requirements, and stage an actual version-checked runtime. Rewrite
install IDs/load edges or build with bundle-relative IDs before inside-out
signing; verify the relocated graph and then hash final signed bytes into the
inventory. Current inventory rejects symlink members, so real-file alias
materialization or an explicitly reviewed safe alias contract is also required.
Only then connect packaged identity to retained-runtime admission/lookup and
perform isolated native A-to-B cleanup/later-worker acceptance; Linux/Windows
need their own loader closure and native evidence. Publication/signing remain
separately unauthorized.

Focused local results: `python3 -B scripts/llama_baseline.py` passed;
`test-llama-baseline.py` 6 tests, `test-runtime-inventory.py` 3 tests and
`test-release-workflow.py` 47 tests all passed. These are metadata/fixture and
regression checks, not a build, native package install, model qualification or
cross-platform acceptance. No direct git plumbing was used; the decision ledger
helper reported no active autonomous run, so the decision is recorded here:
keep validation at the existing release metadata/test gate rather than embedding
a lock into payload inventories that do not yet contain llama.

## Offline opt-in Darwin staged loader reports (2026-11-03)

Added `scripts/darwin_loader_closure.py` and 15 offline tests using synthesized
text fixtures based on the recorded Homebrew graph. The standalone explicit
`--staged-root --loader-reports` command leaves archive builders, mandatory
workflow gates, baseline qualification and runtime lookup unchanged. It checks
static dependencies plus caller-declared backend roots, loader/executable paths,
inherited rpaths, exact system allowlisting, missing and ambiguous targets,
unsupported/external paths, and root/symlink containment. Install IDs are not
edges. All five observed backend candidate names appear in the fixture, not as
a claim that all are required or sufficient. No dynamic discovery is claimed.
Caller report freshness and selected backend completeness are obligations, not
facts this offline check can establish. README records the explicit inventory
contract and non-Mach-O resource obligations.

Decision: separate opt-in command rather than extending mandatory signed archive
verification, which currently has no llama payload and invokes native tools.
This avoids changing the existing archive contract or implying dynamic closure.
The decision ledger tool again reported no active autonomous run.

Focused results: loader tests 15, baseline tests 6, runtime inventory tests 3,
release workflow tests 47, macOS release verifier tests 23 all passed; baseline
validator passed. Only inert temporary files/text reports were staged. No live
binary inspection/mutation/copy, native execution, network, signing, credentials,
installation or model startup occurred. Read-only self-review of implementation,
tests and documentation checked that success is labeled supplied-report closure
and explicitly excludes report freshness and dynamic/resource qualification.

Remaining local code: reproducible source/dependency recipe and provenance;
backend/discovery/Metal/TLS/trust/license inventory; bundle-relative IDs/load
edges; safe alias materialization compatible with archive inventory; actual
staged identity and report-to-byte binding; post-signing inventory generation;
packaged runtime admission/lookup and retention integration. Linux/Windows need
platform loader checks and independent build contracts. No actual bundled
payload was built in this slice.

Separate native-resource authorization/evidence: source/dependency acquisition
if network is needed, native toolchains/build hosts and actual per-platform
builds, isolated model/backend/resource qualification and package A-to-B cleanup
with later-worker acceptance, native Windows locking/ACL and Linux installed
acceptance. Signing/notarization credentials and operations, repository/package
publication and any live installation remain separately unauthorized. Fixture
success does not satisfy any of these requirements.


## Opt-in fresh Darwin staged-image collection (2026-11-03)

Feature-gap probe before implementation:
`python3 -B scripts/darwin_loader_closure.py --staged-root . --image-inventory missing.json`
exited 2 requiring `--loader-reports`. This is expected missing capability,
not a bug: the existing command accepts only supplied offline reports and
cannot collect or bind native output to staged bytes.

Implemented separate `scripts/darwin_staged_reports.py` with explicit
`--staged-root --image-inventory --architecture` inputs. The inventory contains
exactly executable, nonempty backends and nonempty images. Shared staged-member
validation handles containment and aliases; fresh in-memory reports feed the
real existing closure validator. The fixed host `/usr/bin/otool -hv` checks thin
64-bit identity, requested arm64/x86_64 architecture and image role; `-l`
collects load commands. A raw magic precheck rejects fat/non-Mach-O inputs.
Only the host inspector runs, never staged code. Subprocesses have 10-second
per-call timeouts, a minimal environment, capped errors and accepted-output
size, with errors/diagnostics failing closed.

All image hashes are captured before collection, each is rehashed after its
tool calls, then all hashes and inventory path resolutions are checked after
closure validation. This binds ephemeral evidence under the caller-owned
stable-stage assumption, not hostile-concurrency safety. No report/hash schema,
source provenance, dynamic discovery/resource completeness or qualification
claim was introduced. The offline CLI and current release/archive contracts
remain unchanged; archive symlink restrictions are not relaxed.

The first new test run exposed only a test-fixture path issue on Darwin
(`/var` versus resolved `/private/var`); resolving the inert temporary root
fixed the stub's pathname matching. No native payload was involved.

Final commands (all from repository root):

| Command | Tests | Result |
| --- | ---: | --- |
| `python3 -B scripts/test-darwin-staged-reports.py` | 15 | PASS |
| `python3 -B scripts/test-darwin-loader-closure.py` | 20 | PASS |
| `python3 -B scripts/test-llama-baseline.py` | 6 | PASS |
| `python3 -B scripts/test-runtime-inventory.py` | 3 | PASS |
| `python3 -B scripts/test-macos-release-verify.py` | 23 | PASS |
| `python3 -B scripts/test-release-workflow.py` | 47 | PASS |
| `python3 -B scripts/llama_baseline.py` | validator | PASS |

114 tests passed, no skips. New tests use inert temporary bytes and injected
host tools/subprocess results, covering full success through the real checker,
both supported architectures, safe aliases, invalid/escaping/missing paths,
duplicate aliases, missing inventories/roots/dependency reports, unsupported
bytes/headers, tool errors/timeouts/diagnostics, mutation during collection,
late mutation of earlier images, retargeted aliases, malformed/external report
edges, and CLI JSON failures. Expected negative-test FAILED diagnostics do not
indicate suite failures. No native tool or installed payload inspection,
network/download, build, signing, model startup, live mutation, policy change,
push or merge was performed.

Residual local work: source/dependency recipe and provenance; selected backend,
discovery/resource/Metal/TLS/trust/license inventories; bundle-relative staged
IDs/edges and archive-compatible alias materialization; actual native collection
acceptance on authorized stable stages; final signed-byte inventory generation
and runtime admission/retention integration. Linux/Windows loader contracts and
native qualification remain separate. No resource/discovery completeness,
source provenance or bundled runtime qualification follows from this slice.


## Mac-first native staged-closure probe — blocked (2026-11-03)

The user directs Mac-first native validation, with Windows deferred. Added only
an opt-in native integration test and documentation; no production collector or
closure policy changes. Read AGENTS, both implementations, both regression
suites and release/llama README before implementation. No live installation,
configuration/state/VM changes, credentials, network, downloaded code, fixture
execution, model startup, push, merge or git plumbing was used. Host compilers
and inspectors alone ran; all authored C and native bytes were temporary.

### Actual native behavior before any proposed fix

Host `uname -a` reported Darwin 25.5.0 arm64. `/usr/bin/xcrun --find clang`
resolved the host compiler under
`/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/clang`.
The minimization probe reported Apple clang 21.0.0
(`clang-2100.1.1.101`), target arm64-apple-darwin25.5.0, SDK 26.5.

The first opt-in run exited 1: 3 tests, 2 assertion failures and 1 error, all
caused by `unexpected report header or diagnostic`, not dependency resolution.
The final run moved the whole stage before collection (the initial probe tried
collection before moving) and reproduced the same result. No production fix was
attempted. The missing-library and external-edge tests intentionally assert
specific failure messages, so this unrelated parser failure cannot count as
successful negative coverage.

Minimized independently in a fresh temporary directory:

1. Author only `int main(void) { return 0; }` in `empty.c`.
2. `/usr/bin/xcrun clang <temp>/empty.c -o <temp>/empty`: exit 0.
3. `/usr/bin/otool -l <temp>/empty`: exit 0, stderr empty.
4. Isolate the first segment block with unindented lines, replace only the
   informational report header and command ordinal to form a one-command
   report, and pass it to the real `parse_report`: same rejection.

The observed block was `LC_SEGMENT_64`, `cmdsize 232`, `segname __TEXT`,
`nsects 2`, followed by unindented `Section` markers for `__text` and
`__unwind_info`. Every other payload line in that block was indented. This
isolates the root cause at `scripts/darwin_loader_closure.py:58-59`: the
all-payload-lines-indented rule rejects normal native section formatting.
The collector had already accepted Mach-O identity and gathered real output;
this is not a missing toolchain, wrong architecture or bad fixture edge.

That rule is also the fail-closed unexpected-header/diagnostic boundary.
Changing accepted parser grammar requires explicit security review, so the user
stop condition was applied rather than broadening it, filtering native reports,
allowlisting new commands, changing SYSTEM paths or weakening assertions.

### Exact final commands and results

All commands ran from the repository root:

| Command | Result |
| --- | --- |
| `CERCANO_DARWIN_NATIVE_TESTS=1 python3 -B scripts/test-darwin-native-staged-reports.py -v` | exit 1; 3 tests in 0.385s; 2 failures, 1 error; no skips |
| `python3 -B scripts/test-darwin-native-staged-reports.py -v` | exit 0; all 3 explicitly skipped without opt-in |
| `python3 -B scripts/test-darwin-staged-reports.py` | exit 0; 15 tests in 0.021s; PASS |
| `python3 -B scripts/test-darwin-loader-closure.py` | exit 0; 20 tests in 0.131s; PASS |

The collector suite's duplicate-JSON and missing-inventory FAILED diagnostics
are expected negative-test output, not suite failures. Native non-Darwin and
missing-toolchain skip branches are implemented but were not executed on this
available Darwin/toolchain host. No x86_64 or Windows native results are claimed.

### Remaining gap

The committed native probe preserves the intended success-after-relocation,
missing-rpath-target rejection and external-non-system rejection contracts.
Those outcomes remain **unverified on real tools** until a separately reviewed
parser grammar fix allows normal segment section markers without weakening
header/diagnostic rejection. The native opt-in suite is deliberately red and
is not added to release gates or the platform-independent suite. No runtime
execution, full dyld semantics, backend discovery/resources, llama provenance,
signing or installed-system acceptance was established. Windows stays deferred.


## Scoped native segment-parser fix — separate blocker remains (2026-11-03)

The historical blocked probe above is retained as evidence. With explicit user
approval for the segment grammar fix, this follow-up reproduced the original
native failure before editing: the opt-in suite ran 3 tests in 0.319s, with 2
failures and 1 error (`unexpected report header or diagnostic`), no skips.
Read AGENTS, the native test, parser, offline closure suite and prior verification.
The exact opt-in variable is `CERCANO_DARWIN_NATIVE_TESTS`, not
`CERCANO_RUN_NATIVE_DARWIN_TESTS`.

Moved payload indentation validation after command identification and the existing
allowlist check. Only an exact unindented `Section` line in `LC_SEGMENT` or
`LC_SEGMENT_64` is newly accepted. No command allowlist, dependency/path/system
policy, collector behavior or native assertions changed. Offline regressions
include a fresh real arm64 `LC_SEGMENT_64` __TEXT excerpt with two section markers,
a synthetic `LC_SEGMENT` counterpart, exact markers rejected in every existing
nonsegment command, and unexpected unindented diagnostics/near-marker text
rejected in both segment commands.

### Follow-up commands and results

All commands ran from this worktree root:

| Command | Result |
| --- | --- |
| `python3 -B scripts/test-darwin-loader-closure.py` | exit 0; 23 tests in 0.194s; PASS |
| `python3 -B scripts/test-darwin-staged-reports.py` | exit 0; 15 tests in 0.022s; PASS |
| `CERCANO_DARWIN_NATIVE_TESTS=1 python3 -B scripts/test-darwin-native-staged-reports.py -v` | exit 1; 3 tests in 0.347s; 2 failures, 1 error; no skips |

Collector duplicate-JSON and missing-inventory FAILED messages remain expected
negative-test output. The segment-specific rejection is fixed, but native
acceptance is **not green** and relocation/dependency rejection outcomes are
still unverified by that suite.

### Independently reduced remaining native failure (not fixed here)

In a fresh temporary directory, authored only
`int main(void) { return 0; }`, compiled using `/usr/bin/xcrun clang`, and inspected
with `/usr/bin/otool -l`; both exited 0 and otool stderr was empty. Split the
output into individual command blocks and normalized only the informational
header and ordinal before passing each block to the real parser. Segment blocks
now passed; the only rejected block was:

```text
file:
Load command 0
          cmd LC_LOAD_DYLIB
      cmdsize 56
         name /usr/lib/libSystem.B.dylib (offset 24)
   time stamp 2 Wed Dec 31 16:00:02 1969
      current version 1356.0.0
compatibility version 1.0.0
```

Passing that isolated report reproduces `unexpected report header or diagnostic`.
A separate in-memory diagnostic probe adding only one leading space to the
`compatibility version` line parses to
`(['/usr/lib/libSystem.B.dylib'], [])`. This identifies another native formatting
incompatibility, not a dependency-policy failure. No such report rewriting is
used in production or the tests. Per the user stop condition, no additional
parser exemption was introduced; this separate grammar issue needs its own
review. The native test remains red and is not a release gate.

No payload was executed. Only host compiler/inspector tools and offline tests
ran, with authored source/native bytes confined to temporary directories. No
network, install, VM/live-state changes, git plumbing, push or merge occurred.
No runtime, signing, provenance, backend discovery/resource, Windows or x86_64
qualification is claimed.


## Scoped native dylib-version grammar follow-up — green (2026-11-03)

Continued the segment-only fix from ce72a1ea859a; prior blocked results above
remain historical evidence. Before editing, ran
`CERCANO_DARWIN_NATIVE_TESTS=1 python3 scripts/test-darwin-native-staged-reports.py`:
exit 1, 3 tests in 0.348s, 2 failures and 1 error, no skips. All still encountered
`unexpected report header or diagnostic` before dependency resolution.

Built the existing locally authored native fixture in its temporary directory
and printed **complete** real `/usr/bin/otool -l` output for both executable and
dylib (not just an excerpt). Both inspections exited 0. The executable contained
unindented compatibility versions 0.0.0 for the fixture load and 1.0.0 for
libSystem; the dylib contained 0.0.0 in LC_ID_DYLIB and 1.0.0 in its libSystem
load. Current versions were indented (0.0.0 and 1356.0.0). Section markers were
already handled by the prior fix. No other unindented metadata blocker appeared.
Temporary sources/bytes were cleaned up; no compiled payload was executed.

New recognition is restricted to existing LOADS commands and LC_ID_DYLIB, with
full-line grammar `(?:current|compatibility) version [0-9]+\.[0-9]+\.[0-9]+`.
There is no general whitespace relaxation, new command/system allowlist entry,
or dependency/path/security policy change. Existing command identification,
allowlist, sequential ordinal, cmdsize and string-field checks remain intact.
The offline suite includes the real libSystem load excerpt (only header/ordinal
normalized), identity/load variants and unindented current-version coverage;
wrong-command fields, malformed numeric versions, suffix diagnostics and
arbitrary unindented diagnostics remain rejected. Existing envelope regressions
also remain green. Native assertions and collector code were not changed.

Commands run from the worktree root after the fix:

| Command | Result |
| --- | --- |
| `python3 scripts/test-darwin-loader-closure.py` | exit 0; 26 tests in 0.187s; PASS |
| `python3 scripts/test-darwin-staged-reports.py` | exit 0; 15 tests in 0.035s; PASS |
| `CERCANO_DARWIN_NATIVE_TESTS=1 python3 scripts/test-darwin-native-staged-reports.py` | exit 0; 3 tests in 0.415s; PASS, no skips |

Collector duplicate-key/missing-inventory FAILED lines are expected negative
case output, not suite failures. Real relocated-stage closure now passes and
the missing-library/external-dependency cases reach their intended rejection
messages. No further scope blocker was encountered. This establishes only the
local arm64 fixture inspection/closure behavior, not runtime/dyld execution,
x86_64/Windows, signing, source provenance, backend discovery/resources or release
qualification. No network, downloaded execution, installation, live-state
changes, git plumbing, push or merge was used.


## Disposable macOS acceptance VM provisioned — setup pending (2026-11-03)

Supersedes only the historical absence of a macOS runner above; **native package
acceptance is not complete**. Read AGENTS.md and this effort's spec/plan/verification.
Installed Parallels CLI 27.1.0 (58756) supports Apple Silicon IPSW provisioning.
Authoritative workflow: https://kb.parallels.com/125561 documents CLI restore,
custom plain disk and subsequent interactive language/account setup. Fully
unattended Setup Assistant was not established.

VM: `cercano-native-acceptance-20261103-203221`, UUID
`{d5633dbc-c8a1-49fc-a420-df6d5d60d952}`, currently **running**. Dedicated root:
`/Users/bryancostanich/Parallels/cercano-native-acceptance-20261103-203221/`;
bundle beneath it has the same name plus `.macvm`. Resources: 4 CPUs,
8192 MiB RAM, 102400 MiB (100 GiB) plain virtual disk, shared/NAT network.
The dedicated root currently occupies 41 GiB, including the restore image.
Existing Windows 11 UUID `{5cd689b0-153c-47d4-943e-935aa7545684}` remains
suspended; no commands targeted it.

Commands (all VM changes target only the new guest):

```sh
ROOT=/Users/bryancostanich/Parallels/cercano-native-acceptance-20261103-203221
VM=cercano-native-acceptance-20261103-203221
prlctl create "$VM" -o macos --no-hdd --restore-image "$ROOT/UniversalMac_26.5.2_25F84_Restore.ipsw" --dst "$ROOT"
prlctl set "$VM" --device-add hdd --type plain --size 102400
prlctl set "$VM" --memsize 8192 --cpus 4
prlctl set "$VM" --shf-host off --shf-host-defined off --shared-clipboard off
prlctl set "$VM" --sh-app-host-to-guest off --sh-app-guest-to-host off --smart-mount off --shared-cloud off --share-host-location off --auto-share-camera off
prlctl start "$VM"
prlctl start "$VM"
open "$ROOT/$VM.macvm"
prlctl capture "$VM" --file "$ROOT/guest-setup.png"
prlctl exec "$VM" /usr/bin/sw_vers
```

Creation/resource/isolation commands succeeded. First start failed during restore;
second start succeeded. The later `parallels.log` explicitly records restore
**100%, Installation succeeded** at 21:29:11, superseding its earlier failure.
Console capture after opening the bundle shows **Language** and language choices
(verified through Vision OCR using the installed Swift SDK, no host tool install).
`prlctl exec` exits 255, unable to open a guest session; GuestTools not installed.
This is the actual remaining interactive setup boundary, not a missing VM/license.
No guest test tools or packages installed, guest account created, or host
permissions/security altered. Host folders/profile/clipboard/apps/cloud/location
and SmartMount sharing are off. Generic automatic gamepad sharing remains on;
no installed CLI disable switch found. This is not a hardened sandbox claim.

Restore provenance: downloaded bytes came from the official Apple URL
`https://updates.cdn-apple.com/2026SpringFCS/fullrestores/140-24263/B95838F0-6815-4F0B-A039-156526C081AD/UniversalMac_26.5.2_25F84_Restore.ipsw`.
Third-party IPSW.me discovery supplied the matching-host 26.5.2/25F84 URL
(metadata says signed=false; not treated as an authoritative signing verdict).
Apple's Virtualization catalog API failed to load; Apple's downloaded MESU
catalog advertised newer 27.2, not selected. Official CDN response is HTTP 200,
19769902281 bytes. `stat` and `shasum -a 256` independently rechecked the file:
`065abd295a1a456a46c1155217eab92ee95816520ec9aeed83f249f074f68a04`, matching
the official response's `x-amz-meta-digest-sha256`. No other restore payload used.
Dedicated root retains headers, metadata, catalog, download/provision/isolation/
start/retry logs, console capture, helper sources and `verified-provenance.txt`.

Guest access: open the `.macvm` bundle above, finish Setup Assistant using a
disposable local account (no Apple ID/host credentials), then use guest Terminal.
Parallels Tools can be installed **inside this guest** for supported CLI access;
SSH is not enabled and no guest IP/session is verified. Next concrete guest
acceptance-baseline command, not yet run:

```sh
/usr/bin/sw_vers && /usr/bin/uname -m && /bin/df -h /
```

Then prepare guest-only Homebrew/test tools and local fixture package sources
before the required A-to-B install/upgrade/cleanup/later-worker rehearsal. No
Homebrew install/upgrade/session-continuity result or platform support is claimed.


## Disposable macOS VM focused validation — 2026-11-03

This continues the VM provisioning record above: guest execution now works through
`prlctl exec` as disposable user `test` (uid 501) on VM
`{d5633dbc-c8a1-49fc-a420-df6d5d60d952}`. Fresh version probes observed macOS
26.5.2 (25F84), arm64, Homebrew 7.0.12, Go 1.27.3, Python 3.15.2,
CMake 4.4.5, Apple clang 21.0.0 (clang-2100.1.1.101), and CLT SDK
`/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk`. This establishes a usable
focused macOS test environment, not native installed-upgrade acceptance or
completion of Phase 1 environments across all platforms.

### Snapshot provenance and privacy boundary

Guest base: `/Users/test/cercano-validation/65973c75f848a10a2d05d514ca16b11854f69d07`,
with source under `checkout`. The prior transfer was a filtered archive of commit
`65973c75f848a10a2d05d514ca16b11854f69d07` (2,522 paths, 4,499,699 bytes), SHA256
`dedeadbe6d70986424170e6b082566e37695e2ee721016acdc24daf0cc26258a`.
Its omission of the required go:embed fixture caused the earlier worker and
agentclient setup failures; the minimal compaction compile probe reproduced it.

With explicit authorization, restored **only**
`source/server/internal/compaction/testdata/real_conversation.json` using
`git show 65973c75f848a10a2d05d514ca16b11854f69d07:source/server/internal/compaction/testdata/real_conversation.json`
into the private guest checkout via bounded base64 transport. The contents were
not displayed or published, and no host live conversations or substitute fixture
were used. Snapshot identity is **original filtered archive + exact committed
fixture overlay**, not an unfiltered checkout. Overlay size: 6,055,949 bytes;
committed-source and guest SHA256 both
`8a753cb765a1c7c621e39fdc387d64551cbe466899a44b4419a9588642861787`.
Guest archive and overlay hashes were rechecked unchanged after testing. No source
patches were made; only this tracked documentation file changed on the host.

### Isolation, exact commands and observed results

Reused the prior runner's short isolated fixture directories, as `test` through
`/usr/bin/env -i`:

```sh
HOME=/tmp/cnp.h8sjTc/home
XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config
XDG_CACHE_HOME=/tmp/cnp.h8sjTc/cache
XDG_DATA_HOME=/tmp/cnp.h8sjTc/data
TMPDIR=/tmp/cnp.h8sjTc/tmp
GOCACHE=/tmp/cnp.h8sjTc/gocache
GOMODCACHE=/tmp/cnp.h8sjTc/gomodcache
GOWORK=off CERCANO_DISABLE_CREDENTIALS=1
PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin
CERCANO_DARWIN_NATIVE_TESTS=1 PYTHONDONTWRITEBYTECODE=1
```

macOS/Python additionally supplied `__CF_USER_TEXT_ENCODING=0x1F5:0x0:0x0`
and `LC_CTYPE=C.UTF-8`, recorded in each log. Driver command timeout 2400s;
test binaries use 120s. From guest `checkout/source/server`, executed:

```sh
grep -n '^func Test' pkg/agentclient/connect_only_test.go pkg/agentclient/connect_only_isolation_test.go
go test -count=1 -timeout=120s -v ./internal/compaction -run '^$'
go test -list '^(TestDialExistingAutomaticRecoveryCredentialIsolation|TestDialExistingConnectOnlyLoopbackRecovery|TestUnavailableExistingNeverLaunchesAndDefaultStillLaunches)$' ./pkg/agentclient
go test -count=1 -timeout=120s -v ./internal/worker -run '^(TestInstalledRetentionCharacterization|TestInstalledRetainedBundleWorker|TestPinnedFixtureSubprocessGenerations)$'
go test -race -count=1 -timeout=120s -v ./internal/worker -run '^(TestInstalledRetentionCharacterization|TestInstalledRetainedBundleWorker|TestPinnedFixtureSubprocessGenerations)$'
go test -count=1 -timeout=120s -v ./pkg/agentclient -run '^(TestDialExistingAutomaticRecoveryCredentialIsolation|TestDialExistingConnectOnlyLoopbackRecovery|TestUnavailableExistingNeverLaunchesAndDefaultStillLaunches)$'
go test -race -count=1 -timeout=120s -v ./pkg/agentclient -run '^(TestDialExistingAutomaticRecoveryCredentialIsolation|TestDialExistingConnectOnlyLoopbackRecovery|TestUnavailableExistingNeverLaunchesAndDefaultStillLaunches)$'
```

All exited 0. Compaction probe intentionally ran zero tests (compile-only,
not a compaction suite pass). Test listing confirmed all three agentclient names,
including automatic credential-isolation recovery and the unavailable/default
launch case missed by the earlier narrower regex. Worker normal/race each ran
and passed 3 top-level tests, zero failures/skips. Agentclient normal/race each
ran and passed 3 top-level tests plus 3 policy subtests, zero failures/skips.
No retries, source workarounds, or failure masking were needed.

The prior evidence report records Python runtime-inventory 3, loader-closure 26,
staged-reports 15, native-staged-reports 3 tests passed; runtimebundle normal/race
each 5 top-level + 16 subtests; CLI normal/race each 3 top-level + 11 subtests,
all zero skips. Those prior successful groups were not rerun in this continuation.

Evidence: `/tmp/cnp-evidence-65973c75/REPORT.md`, `overlay-provenance.json`,
`post-validation-hashes.txt`, `resume-command.txt`, `resume.py`, and
`logs-overlay/{results.json,counts.json,*.log}` preserve exact commands,
environments, counts and no-skips evidence. Logs-only `overlay-evidence.tar.gz`
was copied back with equal host/guest SHA256 (see `overlay-evidence-digest.txt`);
it contains no fixture. Original logs/archive manifest/exclusions remain adjacent.

### Remaining acceptance gaps

The archive omission blocker is resolved for these selected fixture tests only.
No product/model installation, A-to-B native-manager upgrade/cleanup/later-worker
rehearsal, real installed session continuity, signing/publication, shipped-binary
provenance or broader dynamic-resource acceptance was established. Linux and
Windows native environments were not validated here; no all-platform Phase 1
completion claim is made. No host live agent/config changes, credential access,
Windows actions, publication or push were performed.


## Native Homebrew rehearsal — stopped on convenience blocker (2026-11-03)

Only disposable macOS guest {d5633dbc-c8a1-49fc-a420-df6d5d60d952}, user test/501, was used for brew and executable probes. macOS 26.5.2 arm64, Homebrew 7.0.12 at /opt/homebrew, Go 1.27.3, Python 3.15.2. HOME/XDG/cache isolation is recorded in host launch.py and *-launch.txt; prior source/cache paths were retained. No host brew/agents, Windows, credentials, release publishing, source fixes, overwrite, manual relink, push or merge.

Definitions archive consists of exact git a239f4885874 release/homebrew plus scripts/build-macos-unsigned.sh and scripts/runtime_inventory.py. Host archive-member comparison to git succeeded; guest SHA256 matches c1bfd86854020a925780d7f12b507c6310be7061a29eb0f82e66b6f318ebd5b2. Existing guest snapshot /Users/test/cercano-validation/65973c75f848a10a2d05d514ca16b11854f69d07/checkout supplied actual source. Four native Go builds succeeded with -trimpath -buildvcs=false -ldflags '-X main.version=VERSION'. A/B differ by fixture version injection, not an asserted changed-source revision. Coordinated archives include LICENSE, README.txt, runtime-inventory.json and both binaries. SHA256 A 59b6d5e8eff4ac52c935935cf0c8b47b069399e2fa0c340d0947638d3def1b6b; B 1d1330f190c63a826af6121fdc9ed3817cd1a83ec0493717a8d098cedde22d90.

All three formulas rendered for each version. Fixture copies replace only each generated HTTPS URL line with file:///tmp/cnp-homebrew-evidence/cercano-VERSION-darwin-arm64.tar.gz, preserving checksum and every other byte (verified by fixture-transformation-check.txt). Initial build driver briefly qualified dependency names, reverted before tap staging by test.py; final staged definitions use original bare dependencies. transformation JSON preserves exact final old/new lines. Tap fixture/cercano is guest-local; nothing published or downloaded from a production Cercano release.

Passed: six network-denied sandbox-exec direct binary probes (CLI --help/--version and agent --version at both versions); actual brew CLI A install with no agent formula/command, version-only brew test, CLI help/version, exclusive CLI executable ownership, uninstall CLI; actual brew agent A install with no CLI formula/command, version-only brew test, exclusive agent executable ownership. LICENSE/README and Homebrew metadata are additionally in component kegs; combined inventory intentionally is not. No bare CLI invocation. Final process comm listing contains no cercano/llama process. Final installed agent/CLI versions and both --version outputs are 0.0.901.

Preserved setup/harness failures: initial nohup detach and no-match process grep; core tap clone delay; brew test Bundler rejection of test-owned world-writable gems directory. Removed only o-w on guest /opt/homebrew/Library/Homebrew/vendor/bundle/ruby/4.0.0/gems; CLI brew test then passed. First convenience attempt rejected untrusted local tap; brew trust fixture/cercano succeeded, then retry installed missing CLI dependency alongside preexisting agent, but convenience failed: Error: fixture/cercano/cercano: Empty installation.

Minimal reduced cause: exact committed cercano.rb.in lines 17–19 have an empty install method. Guest Homebrew formula_installer.rb:1249 rejects absent prefix/empty keg; keg.rb:250 ignores metafiles/receipt when deciding emptiness. Captured template/source probes and actual failure are empty-install-template-probe.log, empty-install-root-cause.log and install-convenience-A.log. Initial blocker-minimal-probe.log lacked PATH and its command-not-found outputs are not successful version probes; final-state.py reran explicit isolated-environment version/state probes correctly. One template-path typo is preserved in empty-install-root-cause.log and corrected in empty-install-template-probe.log. No source/formula fix attempted; stopped this slice as instructed.

NOT EXECUTED after blocker: A-to-B brew upgrade, brew cleanup, successful convenience installation/ownership test, final uninstall marker-preservation test, repository-lag/partial-upgrade test, old-monolithic-link migration. migration.py and other prepared scripts are not execution evidence. Both fixture versions were built, but B was not installed. Synthetic config/conversation marker hashes remain unchanged at stop, NOT proof of post-uninstall preservation. Agent/CLI A remain installed for inspection. No live sessions/models, session continuity, later-worker retention, full llama delivery, signing/notarization or distribution-trust claim.

Evidence on guest and host: /tmp/cnp-homebrew-evidence. Host guest/ contains copied logs/results/scripts/formulas and archive inventories; large local binaries/fixture archives remain guest-side. guest-evidence.tar.gz SHA256 f2ebb8315f6d4107f38e175c301ca5134fd400ea068aed29718638a588473de8 equals guest copy (snapshot before this report). Exact commands/results are in build-results.json, test-results.json (including failures/retries), offline-results.json, stopped-results.json, *.log and launch/helper scripts. No source edits or fixes are part of this rehearsal.

## Native Homebrew fixed-template continuation — 2026-11-04

### Scope and exact inputs

Used **only** disposable macOS VM `{d5633dbc-c8a1-49fc-a420-df6d5d60d952}`
as `test`/501 through the existing transport. Reused the preceding isolated
HOME/XDG/cache environment, `/opt/homebrew`, trusted guest-local tap
`fixture/cercano`, and real 0.0.901/0.0.902 archives with the previously recorded
SHA256 values. These binaries have the same application source and version-only
ldflags; this is package lifecycle evidence, not changed-source compatibility.
No root ownership correction or new trust/security change was needed.

Transferred exact `release/homebrew` renderer/template set plus
`scripts/runtime_inventory.py` and `scripts/build-macos-unsigned.sh` from
**`7d7aa262f577299787360c374d528c619ebf403e`**, verified to contain both
`f3bcc2158f84` and `1fa4874f4b30`. Every regular archive member was compared to
`git show` at that SHA. Definitions tar SHA256:
`27d13de48f3b70f83238179049ab7a5f7e3c78c68a8c84fd1e0b700903cbb250`,
confirmed guest-side. Both versions were freshly rendered with that set into
`/tmp/cnp-homebrew-evidence/resume-rendered-VERSION`. Fixture copies in
`resume-fixture-VERSION` replace **only** the generated HTTPS URL line with
`file:///tmp/cnp-homebrew-evidence/cercano-VERSION-darwin-arm64.tar.gz`.
Reverse-replacement assertions establish all other bytes unchanged; checksums and
bare dependency names were preserved. Exact old/new lines are recorded in
`resume-transformations-VERSION.json`. Local tap commits/pulls were normal git
operations within this guest; nothing was published.

### Actual native lifecycle outcomes

All **52 logged guest commands exited 0**, with strict filesystem/marker
assertions also completing:

- Actual `brew install --formula fixture/cercano/cercano` at A succeeded with
  preexisting A components. Fixed convenience keg contains nonempty
  `share/cercano/README.txt`, owns **zero executable files/commands**, and passes
  `brew test`. Both component formula tests passed as well.
- Normal coordinated command (not atomicity proof):
  `brew upgrade --formula fixture/cercano/cercano fixture/cercano/cercano-agent fixture/cercano/cercano-cli`.
  All three advanced 0.0.901 → 0.0.902; each B `brew test` passed. Both direct
  `--version` outputs reported 0.0.902. At A, B and after cleanup, assertions
  required the agent keg's only executable to be `cercano`, CLI keg's only
  executable to be `cercano-cli`, convenience's executable set to be empty, and
  both `/opt/homebrew/bin` links to resolve to their exclusive component owners.
- Targeted `brew cleanup fixture/cercano/cercano fixture/cercano/cercano-agent fixture/cercano/cercano-cli`
  removed all three A kegs and their isolated Homebrew archive caches, reporting
  approximately 242.3 MB freed. Both B commands and ownership remained correct.
- Uninstalled convenience, then components using ordinary `brew uninstall`.
  Both command links were absent. SHA256 assertions against the original
  `marker-hashes.json` proved both synthetic config/conversation markers remained
  byte-identical **after uninstall**, not merely at the earlier blocker.

### Separate old-monolith ownership transition

Also completed a separate **layout fixture**, not a historical-hook compatibility
claim: derived an A `Cercano` formula from the current agent template, changing
only class identity and installing both binaries. This intentionally omits the
historical restart hook; exact formula is `resume-old-monolith-fixture.rb`.
Installed it normally with both commands owned by `cercano`, then staged the
exact fixed split B fixture and ran only
`brew upgrade --formula fixture/cercano/cercano`. Homebrew installed both B
component dependencies and upgraded convenience successfully (exit 0), without
`--overwrite`, manual unlink/link, or repair. The subsequent **strict** ownership
assertions, all three formula tests, targeted cleanup, and version probes passed;
old monolithic A keg was removed. Initial migration state logging used tolerant
`|| true` probes solely to preserve output; the later strict assertions, not that
logging command, establish success. No unresolved migration failure was observed
for this fixture/Homebrew version. This does not cover every historical release,
rollback, interrupted migration, mixed tap state or partial-upgrade ordering.

Finally uninstalled all three formulas again; strict assertions required their
Cellar roots and both command links absent. Both synthetic markers still matched
the original hashes (`resume-markers-final-uninstall.json`). Test archives, local
tap/trust and evidence remain in the guest; no test formula remains installed.

### Hooks, preserved evidence and boundaries

Exact transferred templates have no process launch/restart hooks. Only product
invocations requested were `--version` (direct and component `brew test`), never
bare CLI or agent startup. Continuous 0.2-second `ps pid,comm` samples span both
drivers; sampled product entries are consistent with those short version probes,
and final process listing has no product/model process. No hook-launched product
process was observed; comm sampling is **not exhaustive transient-process
tracing** and cannot independently prove every short-lived argv. No model,
real session, host brew/config/agent, Windows, credential, publication or security
change occurred. No source fix/workaround was needed or applied.

Original empty-install/failure logs and original `guest-evidence.tar.gz` remain
untouched. New host and guest evidence lives under
`/tmp/cnp-homebrew-evidence`: `resume.py`, `resume-finalize.py`, launch files,
`resume-results.json`, per-command logs (including full isolated environments),
ownership/marker results, `resume-main-process-samples.log`,
`resume-process-samples.log`, provenance and definition-member hashes.
Host copy is `resume-guest/`; `resume-evidence.tar.gz` snapshot SHA256 matches
host and guest:
`16f0235cebe17c08dc81800a623cba4227d16aad945bf13459ffa0f4f0cf5c68`.
Additional host command ledger/comparison/fetch helper were copied guest-side
following that snapshot. `resume-host-commands.txt` records exact transport/build
commands and retained harness failures: ad hoc non-isolated preflight lacked brew
PATH (exit 127); first host extraction hit Python 3.9's unsupported
`extractall(filter=...)`, then succeeded with explicit safe regular-file copying.
Neither was a product failure or masked native result.

**Bounded package ownership/install/upgrade/cleanup/uninstall acceptance passed
for these local macOS fixtures.** Remaining gaps: production/full bundled llama
runtime and resource delivery, signing/notarization/distribution trust,
changed-source compatibility, repository-lag/partial upgrades, interrupted or
historical-hook migration/rollback, other native platforms, and installed
session/reconnect/later-worker continuity. Those are not implied by this result;
session continuity was explicitly outside this rehearsal's goal. No plan or
source file was changed in this continuation.


## Native macOS running-process survival / later-worker retention characterization (2026-11-04)

Executed only on disposable guest `{d5633dbc-c8a1-49fc-a420-df6d5d60d952}`,
macOS 26.5.2 arm64, as `test` (uid 501). This is an observed native
**retention gap**, not production acceptance or host-routed conversation continuity.
No production source was changed and no full-plan acceptance item is closed.

### Isolation, preflight and retained harness blocker

Read the prior verification, `resume.py` and existing lifecycle logs; did not rerun
that broad lifecycle. Preflight listed guest packages (no test formulas installed),
processes, original synthetic marker hashes and archive hashes. Used existing
same-source, version-only A `0.0.901` / B `0.0.902` archives and fixed local tap
formulas; only disposable fixture-tap staging used direct git. Archive SHA256:

- A: `59b6d5e8eff4ac52c935935cf0c8b47b069399e2fa0c340d0947638d3def1b6b`
- B: `1d1330f190c63a826af6121fdc9ed3817cd1a83ec0493717a8d098cedde22d90`

All product processes used a new synthetic HOME/config/data/cache/tmp beneath
`/tmp/cnp-macos-update-retention`, an allowlisted environment,
`CERCANO_DISABLE_CREDENTIALS=1`, and sandbox denial of non-loopback networking.
No host product/config was accessed; no model inference, cloud call or download
was requested. Agent config disabled llama-server/mistralrs backends, used
`open_only`, no cloud profiles and unreachable loopback Ollama port 1. Startup
still printed a default local model name but did not run an inference/model server.
The actual agent log confirms credentials disabled and Ollama unavailable.

The first install attempt, with brew also pointed at the new empty config,
failed exit 1: `Refusing to load formula fixture/cercano/cercano-agent from
untrusted tap fixture/cercano.` The convenience formula alone had been trusted;
the failure occurred before any formula installation. Its attempted uninstall
also exited 1. These harness logs/scripts/results are preserved in guest
`attempt1/`. Brew printed `Trusted formula fixture/cercano/cercano` in that
new synthetic config before refusing the dependency; no explicit `brew trust`
command or system security change was made. Corrected only brew's
HOME/XDG paths to the **established synthetic fixture environment**
`/tmp/cnp.h8sjTc/{home,config,data,cache}`; product isolation remained new.
The bounded install/upgrade/retention run then completed normally.

### Native observations

1. Installed A using ordinary `brew install --formula fixture/cercano/cercano`.
   The selected executable resolved to
   `/opt/homebrew/Cellar/cercano-agent/0.0.901/bin/cercano`;
   `--version` returned `cercano v0.0.901` and installed SHA256 matched archive
   member `081a25b496774375f773095330369142da4438f85211ea6f7e3b88b091b7ba4f`.
2. Started ordinary installed `cercano agent`, PID **23131**, on free port
   **49264**, plus actual installed `cercano worker --socket
   /tmp/cnp-macos-update-retention/a.sock`, PID **23136**. Both were ready:
   TCP/Unix probes received HTTP/2 SETTINGS (`000006040000000000000500004000`).
   These are transport readiness checks, not turn execution or authenticated RPCs.
3. Independently staged B, then ordinary `brew upgrade --formula` for the three
   test formulas. Both A PIDs remained alive and transport-ready; all three A
   kegs and A executable still existed immediately after upgrade (automatic
   install cleanup disabled by established fixture environment).
4. Ran targeted `brew cleanup` for only `fixture/cercano/cercano`,
   `fixture/cercano/cercano-agent`, `fixture/cercano/cercano-cli`. Exit 0;
   **all three A kegs and the original A executable path were removed**.
   The same A agent and worker PIDs still responded with HTTP/2 SETTINGS;
   `ps` recorded their original A argv after removal.
5. Attempted a fresh worker at the exact original A executable path.
   **Desired retention assertion FAILED (expected characterization result):**
   `[Errno 2] No such file or directory:
   '/opt/homebrew/Cellar/cercano-agent/0.0.901/bin/cercano'`.
   Running A surviving unlink did **not** retain a launchable A generation.
6. Homebrew-selected `/opt/homebrew/bin/cercano` resolved to
   `/opt/homebrew/Cellar/cercano-agent/0.0.902/bin/cercano`;
   `--version` returned `cercano v0.0.902`, installed SHA256 matched B archive
   member `5169f9f84ab2a062a2b20f62ecca990c248677bff127153a7b719951d241b77a`.
   Started that manager-selected command with `worker --socket .../b.sock`,
   PID **23407**; HTTP/2 SETTINGS readiness passed. Here “manager-selected” means
   the Homebrew command link, **not** an A agent spawning a turn worker via its
   production worker-admission path.

### Teardown, evidence and limits

Only the three owned product PIDs were terminated (agent drained normally,
workers exited on SIGTERM). Ordinary targeted uninstall removed convenience then
components; strict assertions verified all test Cellar roots and both command
links absent. Original two synthetic markers and new config/conversation markers
were SHA256-identical before/after cleanup and final uninstall. Final results
assert every owned PID exited. No generic pkill, manual retention copying,
development launcher, forced links or direct Cellar mutation was used.
Individual command timeout was 600 seconds; each readiness window 30 seconds.
Successful characterization took 12.28 seconds before teardown; guest `DONE`
records total including teardown. Both attempts and orchestration were bounded.

Host and guest evidence directories: `/tmp/cnp-macos-update-retention`.
Host `guest/` contains guest scripts, synthetic config, per-command logs,
`results.json`, `attempt1/` and process/marker observations; `host.py`,
`guest.py`, preflight/launch logs and `evidence-digest.txt` preserve orchestration.
Original `/tmp/cnp-homebrew-evidence` evidence remains preserved (only its
explicitly disposable fixture-tap working tree/commits were staged).
Host `guest-evidence.tar.gz` matches guest `/tmp/cnp-macos-update-retention.tar.gz`
SHA256 `4ea644daf64e98347af7f58ff39a63b0cc8938a6e529f7efc8c619094b8dfa7b`.

This establishes native immediate PID/socket survival and the missing later-A
executable, plus a fresh manager-selected B worker. It does **not** establish
host-routed conversations, in-flight turns, reconnect continuity, A-agent later
worker dispatch, retained runtime resources, changed-source compatibility,
long-duration survival, bundled llama delivery, signing/publication, rollback,
or other native platforms. No retention implementation/fix was attempted.

## Isolated source-level reduction (2026-11-04; implementation still pending)

Starting worktree was clean on `feat/native-packages-session-safe` at
`e37a26a91e8abb54896e4280db0482d3504bd822`. Read repository instructions,
effort specification/plan, latest native characterization and runtimebundle
README. This continuation completed investigation only, **not** the requested
ordinary-installed retention integration.

Executed from `source/server`, with `GOPROXY=off`, `GOSUMDB=off` and
`CERCANO_DISABLE_CREDENTIALS=1`:

- `go test ./internal/worker -run '^TestInstalledRetentionCharacterization$' -count=1 -v`
  passed. Its temporary copied test helper observed A and B resources. Removing
  A's asset yielded helper exit 93 despite successful binary selection; removing
  A's package yielded `worker policy: lstat .../manager/A: no such file or
  directory` before launch. B still worked. This reproduces the missing retained
  dependency without a host agent/model or native package operation. A passing
  characterization means the **gap was reproduced**, not fixed.
- `go test ./cmd/cercano -run '^TestRunAgentStartupAdmissionBeforeConfigAndServices$' -count=1 -v`
  passed both temporary-store cases. An unprepared store failed generation
  authority admission; a compatible prepared store without the validated binding
  failed the service-parity gate. Neither case loaded config or started services;
  the test compared durable fixture bytes before/after.

Source inspection confirms that `productionStartupPolicy.admit` performs
read-only coexistence preflight, while `validateActivation` additionally requires
an exact validated generation/config/store binding. Simply switching the ordinary
startup policy or copying an executable does not satisfy that contract. These
checks must remain intact. Existing ordinary-state conversion is not authorized
by this probe; documented offline store upgrade requires separate approval.
This is not evidence that all implementation is blocked: prepared-state,
fail-closed integration remains work to do.

No production code or test code was changed. In particular, no new actual
agent-dispatch A-removal/B-eligibility test was implemented or run. The existing
helper test is a lower-level reduction, not that acceptance seam. No full suite,
build, native install/VM, live config, credential, agent/model, publication or
push operation was performed in this continuation. No deletion was implemented.

Next prerequisites: connect installed inventory/publication to the existing
validated host admission and session routing; exercise the real dispatch runner
with isolated A/B fixtures and prove pinned failure never falls back to PATH;
then use separately authorized disposable native testing with compatible prepared
store/config and a protected shared-modelhost fixture (or an explicitly
remote-only configuration). Package-payload retention alone will not establish
external llama executable/library/resource closure. That closure, changed-source
compatibility, reconnect and actual model-backed turn acceptance remain separate
unproven requirements. The existing worktree is retained; no landing is requested.

## Opted-in native Homebrew agent-keg retention (2026-11-05)

User approved normal formulas plus persistent
`HOMEBREW_NO_CLEANUP_FORMULAE=cercano-agent`, retaining inactive kegs for now.
Starting worktree was clean on `feat/native-packages-session-safe`, HEAD
`65733c5c7ec60e1911a8982c833ea1ee05934c6f`. Read `AGENTS.md`, worker-selection
documentation, latest verification and preserved retention/Homebrew evidence.
Only existing disposable macOS guest `{d5633dbc-c8a1-49fc-a420-df6d5d60d952}`
was used for native package operations (macOS 26.5.2 arm64, `test`, uid 501;
Homebrew 7.0.12). No host Homebrew/config or live Cercano was modified.

### Preflight, source expectation and persistent scope

Captured full guest `ps user,pid,ppid,comm,args`, installed versions, command links,
fixture tap status and existing system/prefix/test-user `brew.env` candidates.
No test formula or Cercano/llama test process was present; no unowned process was
stopped. Both disposable tap worktrees were clean before staging. Recovered the
existing `resume-fixture-0.0.901` / `resume-fixture-0.0.902` formulas and checked
preserved archive hashes instead of building or fetching new product artifacts.
Only those disposable local-tap formulas were staged/committed/pulled; no
production tap or publication was touched.

Guest Homebrew source inspection: `/opt/homebrew/bin/brew:128-180` loads system,
prefix, then user `brew.env` (with optional system priority); user location is
XDG_CONFIG_HOME, then HOMEBREW_XDG_CONFIG_HOME, else HOME/.homebrew.
`Library/Homebrew/cleanup.rb:417-422` matches the exclusion by formula name or
alias; automatic cleanup filters excluded formulas (`:462`), and explicit
arguments refuse cleanup (`:519-523`). Thus upgrade remains allowed while both
automatic and explicitly targeted cleanup should retain agent A. This is not
`brew pin` or protection against explicit uninstall.

Persisted exactly one line in the previously absent guest file:

```text
/tmp/cnp.h8sjTc/config/homebrew/brew.env
HOMEBREW_NO_CLEANUP_FORMULAE=cercano-agent
```

File SHA256: `144bc621160fd50f59a0e966fa04abfb61a3adfce080fe40cd533416ad1631f2`.
`brew-env-before.json` records that it did not exist. Existing trust and other
settings were preserved. **Scope is only the established synthetic fixture user
context with `XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config`, not guest-wide, not host,
and not the guest's default shell.** `brew config` in that context reported
`HOMEBREW_NO_CLEANUP_FORMULAE: cercano-agent` in both drivers. The setting was
not passed in the command environment; Homebrew read the persistent file.
It remains in place, including during all cleanup and owned-process teardown;
no restoration was needed or performed. This is a bounded manual guest setting,
not product setup automation or a new architecture.

### Exact native commands and observed results

Drivers used `/usr/bin/env -i` and bounded `prlctl exec` transport targeting only
the UUID above. Exact transport, scripts, argv/environments, command output and
results are preserved under the evidence paths below. Each brew subprocess used
this allowlisted environment (TMPDIR differs for the continuation):

```text
PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin
HOME=/tmp/cnp.h8sjTc/home
XDG_CONFIG_HOME=/tmp/cnp.h8sjTc/config
XDG_CACHE_HOME=/tmp/cnp.h8sjTc/cache
XDG_DATA_HOME=/tmp/cnp.h8sjTc/data
TMPDIR=/tmp/cnp-macos-update-retention/policy-20261105/tmp
HOMEBREW_NO_AUTO_UPDATE=1
HOMEBREW_NO_INSTALL_FROM_API=1
HOMEBREW_NO_ANALYTICS=1
```

**No `HOMEBREW_NO_INSTALL_CLEANUP`** was set. Commands below use
`/opt/homebrew/bin/brew`; all returned 0:

```sh
brew config
# Stage existing A fixture definitions; git add/commit then pull --ff-only
brew install --formula fixture/cercano/cercano
/opt/homebrew/Cellar/cercano-agent/0.0.901/bin/cercano --version
# Stage existing B definitions in the same disposable fixture tap
brew upgrade --formula fixture/cercano/cercano fixture/cercano/cercano-agent fixture/cercano/cercano-cli
brew cleanup fixture/cercano/cercano-agent
# Successful continuation, without reinstall/reset or changing the exclusion:
brew config
brew cleanup fixture/cercano/cercano-agent
brew cleanup fixture/cercano/cercano fixture/cercano/cercano-cli
/opt/homebrew/Cellar/cercano-agent/0.0.901/bin/cercano --version
/opt/homebrew/bin/cercano --version
brew list --versions fixture/cercano/cercano fixture/cercano/cercano-agent fixture/cercano/cercano-cli
```

1. A installed at `/opt/homebrew/Cellar/cercano-agent/0.0.901/bin/cercano`,
   SHA256 `081a25b496774375f773095330369142da4438f85211ea6f7e3b88b091b7ba4f`,
   device 16777229, inode **445442**. Version output: `cercano v0.0.901`.
2. Direct A worker PID **27724** became Unix-socket transport-ready, remained
   ready after ordinary upgrade, and was then terminated by its owning driver.
   Upgrade output explicitly included automatic `==> Cleanup`, removing
   `/opt/homebrew/Cellar/cercano-cli/0.0.901` and
   `/opt/homebrew/Cellar/cercano/0.0.901`. Agent A remained with identical device,
   inode and hash. Only agent kegs had both A and B; siblings had only B.
3. Explicit `brew cleanup fixture/cercano/cercano-agent` printed
   `Error: Refusing to clean cercano-agent because it is listed in HOMEBREW_NO_CLEANUP_FORMULAE!`
   **but exited 0 on this Homebrew version**. The initial driver incorrectly
   expected exit 1 and recorded an assertion/harness failure. It safely drained
   PID 27724 and retained formulas/exclusion. That failure is preserved, not
   relabeled as a passing full driver. The continuation recovered the same A
   inode/hash and checked the refusal text and actual filesystem, not exit code
   alone. Both explicit cleanup commands completed; A still had inode 445442 and
   the original hash.
4. After cleanup, a **fresh direct canonical A worker**, PID **28259**, became
   transport-ready. Version probe still returned `cercano v0.0.901`.
5. Public `/opt/homebrew/bin/cercano` resolved to
   `/opt/homebrew/Cellar/cercano-agent/0.0.902/bin/cercano`, SHA256
   `5169f9f84ab2a062a2b20f62ecca990c248677bff127153a7b719951d241b77a`;
   version output `cercano v0.0.902`. A worker launched via that public command,
   PID **28262**, became transport-ready concurrently with fresh A.

All readiness checks sent the HTTP/2 connection preface to the owned Unix socket
and received SETTINGS `000006040000000000000500004000`. Product invocations used
`/usr/bin/sandbox-exec` denying non-loopback network access, synthetic HOME/XDG/
cache/tmp under each driver directory, and `CERCANO_DISABLE_CREDENTIALS=1`.
No ordinary agent was launched in this rehearsal; no inference/model server,
model installation, credential or cloud use was requested. Driver subprocess
limits were 300s, readiness 30s and total work guard 900s. The continuation's
validation completed in 1.831s before teardown.

### Retained state, hashes and limits

All three owned worker PIDs were reaped after targeted SIGTERM, exit -15, with
no escalation needed. Final process observations and `owned_pids_exited=true`
are recorded. No unowned process kill, generic pkill, manual Cellar copying or
mutation, unrestricted cleanup, uninstall, model action, Windows operation,
security bypass or production publication occurred. Retained installed formulas:

```text
cercano 0.0.902
cercano-agent 0.0.901 0.0.902
cercano-cli 0.0.902
```

These are intentionally retained with the exclusion and B-staged disposable tap
so the surviving A path and selected B command remain reproducibly inspectable.
No automatic retirement mechanism is implemented. Original synthetic config and
conversation markers remained identical:
`218589323cbe80b7ed077e3ee36f1663e7cb5f8f4e4ad02c938ad8a5c2c5a6b9` and
`7128e61cd02a6fc2440a495468ebc424d4ca4a81f5ee749dd4ffe311760defb3`.

Preserved source archives (same-source/version-only fixtures from the prior
rehearsal, **predating the installed-worker-policy change**):

- A archive: `59b6d5e8eff4ac52c935935cf0c8b47b069399e2fa0c340d0947638d3def1b6b`.
- B archive: `1d1330f190c63a826af6121fdc9ed3817cd1a83ec0493717a8d098cedde22d90`.

New evidence is additive; prior evidence archives were not overwritten. Host and
guest `/tmp/cnp-macos-update-retention/policy-evidence.tar.gz` are SHA256-equal:
`de27b237a7d70a2c4cae6c3a7d8726541dd699f18e1834bd0dcc8a5402873cd4`.
It contains `policy-20261105/`, `policy-20261105-continue/`, both guest drivers,
preflight/source/launch logs and the fetch helper. Host extracted copy is
`/tmp/cnp-macos-update-retention/policy-guest-evidence/`. Additional post-run
read-only source capture (outside the archive, on host and guest):
`/tmp/cnp-macos-update-retention/policy-source-final.log`, SHA256
`f5bab7dbcd838eb2e5f2b0a105e72823988d490bedf9f8afff8555c4522466f3`.
It preserves numbered `brew:128-180` and `cleanup.rb:410-530,1055-1080` and also
shows runtime formula dependencies excluded from autoremove; that is not a lease
on their old versions or external llama. Host helper commands:
transfer each driver with the preserved UUID-bound transport, invoke
`/usr/bin/env -i PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin HOME=/tmp/cnp.h8sjTc/home /opt/homebrew/bin/python3 /tmp/cnp-macos-update-retention/policy-guest.py`
and the same command with `policy-continue.py`; both driver processes returned 0
(the first driver reports its assertion failure in results). Then run
`python3 /tmp/cnp-macos-update-retention/policy-fetch.py`; exit 0, host/guest
archive digest comparison passed.

This verifies native **retention/launchability with effective external Homebrew
policy**, not actual A-agent later-worker dispatch, turn/session continuity,
protected generation authority, changed-source compatibility, external llama or
resource retention, or explicit uninstall protection. Source-policy fixtures are
separate evidence; the old native binaries do not validate that newer policy.

Focused current-source check from `source/server`, with `GOPROXY=off`,
`GOSUMDB=off`, `CERCANO_DISABLE_CREDENTIALS=1`:

```sh
go test ./internal/worker ./internal/server ./cmd/cercano -run 'TestInstalled|TestProductionCompositionPolicy|TestSelectExecutionMode' -count=1
```

Passed (exit 0): worker 12.944s, server 0.996s, command 1.412s. No full suite,
race rerun or new native binary build was claimed. Documentation-only changes;
no spec/plan, source, formula, setup automation or architectural change.

`git diff --check` passed (exit 0); changed-path inspection showed only
`docs/homebrew-worker-selection.md` and this verification log. Checkpoint is
documentation-only, explicit-path staged, with no push/land; the worktree and
guest fixtures remain retained.
