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
