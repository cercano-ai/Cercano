# Native packages and session-safe upgrades

## Status and purpose

Revised specification for user review. This revision supersedes the previous custom-updater specification in this effort. Historical approvals of self-managed installation, a Go update helper, and a TUF feed do not authorize those features under the revised scope. The existing plan is superseded and must be rewritten after this specification is accepted; it is not an execution checklist for this revision.

Cercano needs straightforward installation and upgrades across macOS, Windows, and Linux without becoming an installer vendor. Native package managers deliver releases. Cercano preserves active sessions across package changes and manages only the runtime generations those sessions require.

This specification defines product behavior and ownership. File layouts, locks, endpoints, retention bookkeeping, and other implementation mechanics belong in the execution plan and code review rather than additional product-design checkpoints.

## Goals

Provide native packages through Homebrew on macOS, Chocolatey on Windows, and APT on Linux. Keep the command-line client and agent independently installable, with a convenient installation of both for everyday users.

Allow package upgrades without silently terminating active sessions or removing the executable and resource dependencies needed by their later worker launches. Compatible new sessions may use a newer generation while existing sessions remain on their original generation.

Clearly distinguish a release announcement, availability through the user's package source, successful package installation, and the version actually serving a session. An installed release is not necessarily an activated runtime.

Avoid implementing downloads, package verification, installation transactions, package rollback, or a second package database inside Cercano. Completion requires native installed-system evidence on all three platforms, not merely successful archive builds.

## Package and release scope

The intended package topology is a dependency-only convenience package named `cercano`, with independently installable `cercano-cli` and `cercano-agent` packages. The agent package does not depend on a client. A standalone client must remain useful with a remote agent without requiring a local agent installation.

CLI and agent packages initially ship as coordinated, tested release sets. This does not establish independent release schedules or promise untested cross-version compatibility. A version-string difference alone is not a reason to reject a connection; actual supported protocol and capability requirements still apply.

The shared model runtime remains within the agent delivery scope initially, rather than becoming another independently released product. Package names do not by themselves rename existing executables. Packaging must preserve the everyday `cercano` entry point and explicitly resolve command ownership without conflicting package files.

Exact dependency declarations and platform-specific package definitions are implementation details to validate against each manager. The packages must remain safe during partial upgrades and when a release has not yet reached every package repository; coordinated publication is not a claim of atomic multi-package installation.

## Ownership boundary

Native package managers own acquisition, package authenticity and integrity checks, installation, dependencies, upgrades, package metadata, and uninstall. Cercano does not overwrite manager-owned files or introduce a self-managed opt-in that bypasses the manager.

Cercano owns runtime selection and retention: preserving the executable/resource bundle needed by a running generation, keeping existing sessions and their workers bound to that generation, admitting new sessions to eligible generations, and retiring generations no longer needed. Retained runtime files are not another managed installation or a permanent package rollback cache.

This boundary applies to upgrades initiated outside Cercano as well as in-app actions. Safety cannot depend on the user always upgrading through the application. Package scripts must not blindly restart agents, kill processes, or remove resources still required by active sessions. Windows in-use executable restrictions must be addressed and tested, not assumed away.

Unknown installation ownership results in guidance, not guessed replacement. Development checkouts and manually extracted archives are not silently enrolled into a custom update system. Package removal must not silently delete conversations, configuration, or credentials; support for runtime continuity during explicit uninstall is not implied by the upgrade guarantee.

## Session and runtime behavior

A compatible package upgrade does not require all sessions to stop. Existing sessions remain on their admitted generation, including later worker starts. New sessions may use the new generation once it is eligible. Retention must cover the actual runtime dependency bundle, not merely an already-running executable.

An incompatible shared-model-runtime change waits to activate until existing sessions depending on the old runtime finish. Package installation may complete in the meantime. An agent requiring the incompatible runtime must remain deferred rather than replacing the old runtime underneath its users. The application must explain that installation succeeded but activation is waiting.

Running incompatible model runtimes side by side is not required initially. Compatible runtime reuse remains allowed. There is no deadline that silently forces cancellation, and a transient idle moment within an active session is not permission to invalidate that session's future runtime needs.

Shared data must remain safe for every generation allowed to access it. Retaining executables does not make an incompatible schema or configuration migration safe. No automatic migration may invalidate an active generation; if safe coexistence cannot be established, the affected activation must remain blocked with a clear explanation. This is a safety constraint, not a choice of database migration mechanics.

Generation cleanup must establish that a generation is no longer needed before deleting it. Failed or uncertain cleanup remains pending; it must not trigger forced process termination. Cleanup never deletes user data or the active generation. Crashes and interrupted operations must fail safely without selecting incomplete runtime files.

## User experience

Update checks are asynchronous and bounded. They must not delay ordinary startup, request elevation, launch a model, or mutate the installation. Help and version commands remain offline and side-effect-free.

Notifications distinguish a newer announced release from one available through the installed package source. Users can view release notes, defer, or initiate the appropriate native-manager action where safe. Otherwise, Cercano provides the appropriate command and explains the expected outcome. It does not collect administrator passwords or invent an always-privileged service to avoid authorization prompts.

User-visible progress distinguishes package installation from runtime activation. It must report installed and running versions separately when they differ, explain deferred activation, and avoid claiming that a successful manager command proves a running session upgraded. Existing sessions do not silently reconnect to a different generation merely to make version reporting match.

Errors must identify the failed boundary: package-manager operation, unavailable package, runtime eligibility, incompatible runtime/data, or pending cleanup. Recovery guidance respects package-manager ownership. Cercano does not silently roll back package files after a runtime failure.

## Platform and trust requirements

Homebrew owns its Cellar, links, metadata, and upgrade transaction. Cercano does not manually switch Homebrew links or perform an application-specific stop-and-restart as an unavoidable upgrade step.

APT owns repository verification and system package installation. Privileged package actions must not launch a root-owned user agent or stop another user's agents. Supporting APT does not imply support for every Linux distribution or package manager.

Chocolatey owns Windows installation and upgrades. Package scope and user identity must be explicit; elevation under another account or execution as SYSTEM must not accidentally install into or control the wrong user's environment.

Use native package verification and the platform signing requirements appropriate to each distribution path. Apple signing/notarization, Windows signing, and repository credentials remain release obligations; they are not replaced by unsigned checksums. A custom TUF client/feed and custom download/staging pipeline are out of scope.

Repository availability, package publication, signing, and installed-system support must be verified before being advertised. Existing templates, experimental archives, or historical audit claims are not proof that a production package source is ready.

## Decisions

### Native package delivery, Cercano runtime retention

The user approved native package managers as the installation and upgrade authority and explicitly rejected building an installer. Cercano retains only the generations needed for runtime continuity. Custom-updater acquisition, verification, staging, and installation machinery is not carried forward merely because it already exists. Existing code is reusable only where it serves the runtime responsibilities in this specification.

### Defer incompatible shared-runtime activation

The user approved waiting for existing sessions rather than requiring concurrent incompatible model runtimes initially.

| Axis | Deferred incompatible activation — chosen | Side-by-side incompatible runtimes |
|---|---|---|
| Behavior | Preserve old runtime until its sessions finish | Old and new runtimes serve their respective sessions |
| Implementation scope | Compatibility checks, dependency tracking, deferred status, safe transition | Separate runtime selection, endpoints, retention, and lifecycle tracking |
| Resource use | One active runtime and loaded model set | Potentially multiple loaded model sets |
| Risk | Long sessions can postpone activation | Resource exhaustion or incorrect runtime attachment |
| Outcome | Session continuity without duplicate model capacity | Immediate access to incompatible new releases |
| Side effects | Installed and active versions can differ temporarily | Greater memory/GPU demand and coexistence test coverage |

This policy governs activation, not permission for the package manager to install a release. The strongest trade-off is that long-running sessions may delay use of an incompatible release.

### Keep this specification at the policy level

The user requested no further deep dive into mechanics during specification drafting. Implementation must preserve these boundaries without repeatedly reopening settled product choices. A genuinely new conflict with these requirements must be surfaced, but routine file layouts and algorithms belong to execution planning.

## Validation and acceptance

Acceptance requires real package install and upgrade rehearsals on macOS, Windows, and Linux in isolated environments. Tests must cover both application-initiated guidance/actions and externally initiated package upgrades. Never test against the developer's live installation, agent, credentials, or production repositories.

Verify the convenience install, standalone agent, and remote-only CLI scenarios; command ownership; partial package upgrades; unavailable repository versions; privilege boundaries; and preservation of configuration, conversations, and credentials.

For a compatible A-to-B upgrade, keep an A session alive through the package upgrade and package-manager cleanup, start a later worker from A successfully, and demonstrate that a new eligible session can use B. Verify that old generation cleanup waits until its dependencies end.

For an incompatible shared-runtime upgrade, demonstrate successful package installation with deferred activation, uninterrupted existing sessions, clear status for blocked new activation, and eventual safe transition after those sessions finish. Verify that incompatible shared-data changes cannot silently invalidate active generations.

Exercise interrupted runtime publication/selection, concurrent launches, insufficient disk space, missing resources, permission errors, failed health checks, and deferred cleanup. Native Windows coverage must include locked executables and paths with spaces or Unicode. Assertions must distinguish package-manager success from actual running-generation readiness.

Code compilation and unit tests alone do not establish cross-platform upgrade support. The execution plan must specify focused unit/integration coverage and native installed-system acceptance, with recorded evidence and explicit limitations.

## Non-goals and authorization boundaries

No custom installer/updater, self-managed enrollment, package ownership takeover, custom update feed, permanent rollback cache, broad process killing, automatic forced cancellation, or initially required side-by-side incompatible model runtimes.

No claim of interruption-free upgrades before the retained runtime bundle and package-manager cleanup behavior have been verified. Existing generation code is groundwork, not proof of the complete installed-package-to-retained-runtime flow.

Specification approval does not authorize implementation. The execution plan requires its own approval. Neither approval authorizes publishing packages or repositories, provisioning credentials, changing existing release tags, altering repository protections, or modifying the developer's live installation. Those operations require explicit authorization.
