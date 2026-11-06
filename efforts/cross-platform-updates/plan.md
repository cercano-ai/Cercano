# Native packages and session-safe upgrades

## Windows replacement execution scope — approved specification amendment

The next execution is limited to Windows ordinary Chocolatey replacement packaging under the 2026-11-06 spec amendment. The phases below are the current Windows checklist, in execution order. Historical cross-platform phases following them preserve prior progress but do not authorize Windows retention, rolling upgrades, or concurrent-version sessions. macOS/Linux requirements are unchanged and are not implementation work in this Windows slice. Terminal windows need not close: only the components being upgraded must exit. No forced termination, custom updater, new privileged service, or per-user executable copies.

## Windows phase 1 — Establish native replacement safety

Objective: verify Chocolatey transaction behavior before choosing hook details or claiming locked-file safety. Files: new isolated fixtures under release/chocolatey/tests/ and recorded effort verification evidence; read existing Windows artifact builders, agentclient discovery/reconnect and Chocolatey lifecycle documentation/source. Tests: harmless fixture packages on a disposable native Windows runner, never the developer's installation. Obtain runner access authorization if none is available; do not start or modify an existing VM merely because it exists.

- [ ] Create an isolated worktree from the approved planning checkpoint and record baseline identity; preserve unrelated worktrees and artifacts
- [ ] Establish an authorized disposable Windows environment and record Chocolatey, OS and architecture versions; mark native checks blocked if unavailable
- [ ] Trace and probe actual upgrade/uninstall ordering, payload replacement and shim handling; do not treat BeforeModify failure as a reliable veto
- [ ] Reproduce running-executable and stopped-but-locked-file replacement using harmless fixture binaries, including concurrent relaunch and inaccessible process information
- [ ] Establish manager-native safe failure and actionable retry behavior without killing processes or silently accepting partial installation; record exact observed old-payload and shim state
- [ ] If ordinary package replacement cannot satisfy the approved safety boundary, report that concrete conflict before adopting a different installer architecture

## Windows phase 2 — Component packages and command discovery

Objective: generate independently installable component packages and a dependency-only convenience package from verified release artifacts. Files: release/chocolatey/ package definitions, builder and tests; scripts/build-windows-release.py and scripts/test-windows-release-build.py only where needed; source/server/pkg/agentclient discovery tests. Tests: package membership, identity/checksum validation, dependencies, command ownership and remote-only startup. Shared model runtime remains within agent delivery scope, but the current ZIP has no bundled llama; record that delivery gap rather than silently treating the two-binary package as complete local-runtime support.

- [ ] Add cercano-agent owning cercano.exe and its public shim, cercano-cli owning cercano-cli.exe and its shim, and cercano depending on both without command ownership
- [ ] Preserve independent component installation and per-user data; package scripts must not launch elevated user agents or enumerate/delete other users' data
- [ ] Implement only replacement/error handling supported by phase 1 evidence, covering all package-owned executables/resources rather than process-name-only detection
- [ ] Test checksum/layout failures, partial component upgrades and repository lag; avoid assuming atomic installation or arbitrary cross-version compatibility
- [ ] Verify separate package directory and shim-based agent discovery, missing/stale shims, and spaces/Unicode paths; preserve remote-only CLI no-launch behavior
- [ ] Document external llama delivery limitations and keep local-runtime qualification pending unless a supported delivery path is established within scope
- [ ] Run focused fixture and interface integration tests, review changes and checkpoint explicit paths

## Windows phase 3 — Exit and reconnect behavior

Objective: distinguish terminal lifetime from executable lifetime and prevent reconnect racing agent replacement. Files: source/server/pkg/agentclient/{client.go,reconnect.go}, source/clients/cli startup/UI and Windows installation documentation only as required by observed probes. Tests: disconnected client, automatic recovery, unavailable endpoint and compatible restored endpoint. Reuse existing behavior where correct; reproduce failures before changing it.

- [ ] Verify CLI exit is required only when its own component is replaced; terminal shells may remain open
- [ ] Probe agent-only replacement with a disconnected CLI, including watcher-driven local relaunch during the replacement interval
- [ ] Keep CLI-open agent upgrades unsupported if existing coordination cannot safely prevent relaunch; document exiting the CLI as well for that case rather than inventing an update service
- [ ] Where supported, verify reconnect after successful replacement and protocol compatibility checks without claiming in-flight turn preservation
- [ ] Add focused regressions for demonstrated defects, preserving remote connect-only behavior and avoiding Windows generation admission/retention changes

## Windows phase 4 — Native acceptance and nonpublishing integration

Objective: prove ordinary package lifecycle and failure reporting before advertising Windows installation support. Files: release/chocolatey/tests/, scripts/test-release-workflow.py, .github/workflows/release-macos.yml, docs/windows-artifact.md and Windows installation guidance. Tests: native install/upgrade/uninstall, preservation and lock matrix; artifact and workflow tests remain distinct from installed-system acceptance.

- [ ] Exercise standalone agent, remote-only client and convenience installation on the authorized disposable runner
- [ ] Perform a real A-to-B replacement with affected components exited; verify selected executable versions, shims and usable new startup
- [ ] Exercise running/locked files, cross-user processes, concurrent relaunch, interruption, insufficient permissions and partial packages; verify errors and recovery guidance without forced termination
- [ ] Verify configuration/conversation/model sentinels and synthetic credentials survive upgrade and uninstall, including alternate elevated account boundaries
- [ ] Integrate package generation and nonpublishing validation into the existing workflow without weakening macOS gates or enabling publication
- [ ] Record exact versions, commands, hashes, test counts, failures and limitations; distinguish same-source fixture upgrades from changed-release compatibility and real inference
- [ ] Obtain independent review, resolve findings, rerun affected focused/native tests and checkpoint completed units
- [ ] Keep signing, publishing, credentials, release tags, live installations and unsupported runtime claims gated separately; do not push or merge without authorization

## Historical cross-platform scope and progress

The following records pre-amendment work. Windows rolling/retention and deferred-activation tasks below are superseded by the Windows phases above, not pending Windows requirements. Existing macOS/Linux work is not reopened by this execution plan.

Execute the revised spec.md, approved by the user in conversation. This plan supersedes the previous custom-updater plan entirely. All tasks are pending; existing code, prototypes and historical audit results are reuse candidates, not acceptance evidence.

Native managers own package delivery. Cercano owns only runtime continuity, selection and retirement. No TUF feed, custom installer, self-managed enrollment, package rollback engine or mandatory stop-all upgrade is authorized. Incompatible shared-runtime activation waits for existing sessions to finish. Routine implementation details are resolved during execution within these approved boundaries, not through another round of product-design questions. Escalate only genuine conflicts with the specification.

Use an isolated feature worktree inheriting the approved planning commit. Delegate bounded mechanical changes and audits, checkpoint tested units with explicit paths, and never push without authorization. Follow the repository self-development guidance and preserve the developer's live installation and running agents. Run focused tests first and interface integration tests when contracts change. Native installed-system acceptance is mandatory before claiming platform support; missing runners or credentials are reported as blockers, never passes. Production publication remains separately unauthorized.

## Phase 1 — Establish the implementation baseline

Objective: identify the smallest safe integration points and reproduce existing behavior before changes. Files: docs/agent/self-dev.md, docs/agent/development-generations.md, release/homebrew/, source/server/pkg/agentclient/, source/server/cmd/cercano/, source/server/internal/worker/, source/server/internal/modelhost/, source/server/pkg/update/, and effort verification notes. Tests: focused existing installed-discovery, pinned-session, worker-generation, modelhost nonreplacement and formula-rendering tests. The inspected development-generation code does not establish installed-package retention or automatic safe retirement.

- [x] Create the isolated worktree from the approved planning checkpoint and record branch/base identity
- [~] Trace installed executable discovery through host admission, reconnect and later worker starts; identify every required binary, library and resource
- [x] Record current package file ownership and everyday command resolution, including standalone CLI remote-agent use
- [x] Trace runtime replacement callers and shared-store/configuration admission; identify unsafe replacement or migration paths
- [x] Run and record focused baseline tests; for observed defects, capture the smallest failing probe before fixing
- [~] Establish disposable native test environments and list missing resources in efforts/cross-platform-updates/verification.md
- [x] Record concrete reuse boundaries; do not merge the old custom-updater branch wholesale
  Local baseline closeout (2026-11-03): focused Go tests now pass via PATH with isolated fixture state; package ownership and replacement/config-admission tracing are recorded in verification.md. The discovery/dependency task remains in progress only for complete per-platform shipped library/resource closure; the source-level inventory and concrete first Phase 2 fixture are recorded. Native environments/acceptance remain blocked/deferred. No source implementation or native upgrade acceptance is implied by these completions.

## Phase 2 — Retain installed runtime generations and preserve session binding

Objective: bridge native installed files into complete retained runtime bundles without taking ownership of package delivery. Files: source/server/cmd/cercano/{development_launch.go,development_current.go,ordinary_generation_host.go}, source/server/pkg/agentclient/{client.go,pinned_session.go,development_identity.go,reconnect.go}, source/server/internal/worker/{host.go,pool.go,spawn.go}, and narrowly scoped runtime-generation support as needed. Existing source/server/scripts/{publish-dev-generation.py,cercano-launcher.py} are reference/reuse candidates, not assumed portable production installers. Tests: installed discovery/reuse, pinned-session, generation replacement, launch integration and retirement tests, plus fixture bundles and concurrent-process integration tests.

- [~] Add failing installed-flow fixtures proving which package removal/replacement events currently break later runtime use
- [~] Implement complete runtime-bundle retention and validation before admission, independent of replaceable manager-owned files
- [~] Make interrupted publication or incomplete resources ineligible; preserve the prior valid runtime selection
- [~] Integrate installed discovery and launch with retained-generation selection while preserving remote-only CLI operation
- [ ] Keep active and reconnected sessions bound to their admitted generation and route their later workers to the same retained dependencies
- [ ] Admit new sessions to eligible newer generations without redirecting existing sessions
- [ ] Track live generation dependencies across hosts, sessions and workers; handle crash recovery conservatively
- [ ] Retire only proven-unused generations, retry failed cleanup without process killing, and protect user data
- [ ] Test simultaneous launches, interrupted publication, missing assets, package-file removal, disk/permission failures and crash recovery
- [ ] Verify Windows-compatible retention and launch behavior with locked files and Unicode/space-containing paths
- [ ] Checkpoint runtime continuity with focused and cross-component integration evidence
  Bounded slice (2026-11-03): caller-inventoried runtimebundle publication and retained worker-policy fixture completed with focused and race tests. Retention/publication tasks remain in progress: installed discovery/admission, complete platform dependency inventory, recovery, and native acceptance are not connected or complete. See the latest verification section and runtimebundle/README.md; no cleanup or package delivery implemented.

## Phase 3 — Defer incompatible runtime and data activation

Objective: allow package installation while preventing activation that would invalidate an existing session's runtime or shared data. Files: source/server/internal/modelhost/{ensure.go,registry.go,server.go,protection.go,holds.go}, source/server/cmd/cercano/development_host_production.go and production startup/admission paths, source/server/internal/worker/runtime_control.go, and existing store/configuration admission code identified in Phase 1. Tests: attachment_lifetime_test.go, ensure_nonreplacement_test.go, engine_holds_test.go, release_retry_test.go and new session-lifetime/activation integration cases.

- [ ] Reproduce incompatible replacement behavior with isolated fake/runtime fixtures before changing it
- [ ] Protect the runtime for the full lifetime of sessions that may need it again, not merely during an active inference call
- [ ] Reuse compatible running runtimes and defer incompatible activation while old session dependencies remain
- [ ] Permit the safe transition after those dependencies finish without a timeout-to-kill policy
- [ ] Prevent shared schema/configuration changes from invalidating any admitted generation; block unsafe activation rather than silently migrating
- [ ] Expose structured eligibility/deferred reasons for the update status layer without introducing blanket version-string rejection
- [ ] Test installed-but-deferred state, long-lived idle sessions, later worker use, interrupted transition and eventual safe activation
- [ ] Checkpoint compatibility enforcement and its integration tests

## Phase 4 — Build native package definitions and remove forced restart assumptions

Objective: supply independently installable CLI and agent packages plus a dependency-only convenience package, with manager-owned installation and no unconditional agent restart. Files: release/homebrew/cercano.rb.in, render_formula.py and associated tests; new release/debian/ and release/chocolatey/ definitions/tests; .github/workflows/release-macos.yml and update-homebrew.yml; source/server/internal/brewrestart/ and compatibility entrypoints where package integration requires changes. Tests: package-content/dependency checks, formula rendering and native disposable install/upgrade/uninstall fixtures. Definitions may be prepared in parallel, but their upgrade safety depends on Phases 2–3.

- [ ] Define nonconflicting package contents and command ownership; preserve the everyday entry point and standalone remote CLI without silently renaming the server executable
- [x] Split Homebrew packaging into CLI, agent and convenience definitions; remove mandatory restart-after-upgrade behavior
- [ ] Add Debian/APT and Chocolatey package definitions using manager-native dependencies, verification and upgrade operations
- [ ] Keep releases coordinated; verify dependency behavior during partial upgrades and repository lag rather than assuming atomic installation
- [ ] Ensure package hooks neither launch root-owned user agents nor stop other users' sessions; reject wrong-account/SYSTEM misuse where applicable
- [ ] Test package-manager cleanup against retained generations, including Windows executable locking
- [ ] Verify uninstall preserves user configuration, conversations and credentials without claiming session continuity during explicit uninstall
- [ ] Adapt or retire obsolete forced-restart paths where necessary without importing self-managed update machinery
- [ ] Checkpoint each platform's packaging with its actual validation evidence and outstanding native blockers

## Phase 5 — Report update availability, installation and activation truthfully

Objective: provide bounded asynchronous notifications and native-manager actions or safe command guidance while keeping installed and session-running versions distinct. Files: source/server/pkg/update/{update.go,update_test.go,no_self_replacement_test.go}, source/server/cmd/cercano/main.go, source/clients/cli/internal/ui/{model.go,settings_page.go,version_identity.go,version_identity_test.go}, plus server handlers and additive source/proto/ definitions only where needed. Tests: fake-manager command/ownership tests, deterministic UI states and real client/server integration tests for changed interfaces.

- [ ] Detect supported package ownership from corroborated installed state; treat unknown/manual/development installations as guidance-only
- [ ] Separate announced release, package-source availability, installed version, running session version and deferred activation
- [ ] Keep discovery bounded, cached and asynchronous; keep help/version offline and side-effect-free
- [ ] Offer release notes, deferral and appropriately authorized native-manager actions; otherwise provide safe explicit command guidance
- [ ] Never collect administrator passwords, run release-provided shell text or overwrite package-manager-owned files
- [ ] Surface package failure, runtime ineligibility, deferred activation and pending cleanup distinctly
- [ ] Preserve truthful status across client reconnects and partial upgrades; do not switch active generations to make version displays match
- [ ] Test unavailable managers, unavailable repository versions, denied authorization, unknown ownership and compatible older peers
- [ ] Run interface integration tests and checkpoint the user experience

## Phase 6 — Rehearse distribution and prove installed-system acceptance

Objective: prove the approved behavior on all three native platforms before support claims or publication. Files: package/native integration fixtures, non-publishing release workflow validation, efforts/cross-platform-updates/verification.md, platform installation documentation, docs/features/cli/README.md and relevant agent docs. Tests: disposable installed systems with fixture/test signing where appropriate; production signing and publication remain separately gated.

- [ ] Rehearse coordinated artifact and package generation without publishing; validate package contents and verification/signing requirements
- [ ] Exercise convenience install, standalone agent, remote-only CLI and package removal with preserved user data on each platform
- [ ] Upgrade A to B through the native manager while an A session remains alive, including externally initiated upgrades and manager cleanup
- [ ] Prove a later A worker still launches, new eligible sessions use B, and A is retired only after dependencies end
- [ ] Prove incompatible runtime installation succeeds with activation deferred, existing sessions remain usable, and transition occurs safely after they finish
- [ ] Verify incompatible shared-data changes are blocked rather than invalidating old readers/writers
- [ ] Exercise partial upgrades, repository lag, multiple users, privilege failures, concurrent launches and interrupted selection/publication
- [ ] Run the native failure matrix including missing resources, disk/permission failures, cleanup retry and Windows locked/Unicode paths
- [ ] Record exact commands, versions, outcomes and limitations; distinguish unit/integration success from installed-system acceptance
- [ ] Update user/operator guidance for deferred activation, recovery boundaries and manager-owned rollback; remove stale custom-updater claims in affected docs
- [ ] Obtain independent review of session safety, package ownership and acceptance evidence; resolve findings and rerun affected tests
- [ ] Checkpoint completed units and final evidence; mark unavailable acceptance checks blocked rather than complete
- [ ] Request separate authorization before any production publication, credentials provisioning, release-tag changes or live-installation changes
