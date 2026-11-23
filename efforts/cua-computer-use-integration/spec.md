# Shared desktop broker and computer-use integration

Status: approved by the user in conversation on 2026-11-22. Implementation remains subject to approval of the execution plan.

## Problem and motivation

Cercano needs general computer use from both its CLI and upcoming Avalonia client. Accessibility is an optimization, not a boundary: supported operations must include screenshot-grounded pixel interaction and explicit foreground interaction where structured controls or background delivery are insufficient. Cua Driver is a candidate shortcut, not a permanent architectural dependency.

Computer control must occur on the explicitly selected client or host, which may differ from the agent's machine. Multiple Cercano clients may share one desktop. Independent input pipelines would interfere with one another, duplicate permission/lifecycle logic, and complicate migration to Avalonia.

## Foundation and scope

Implementation must build on feat/client-workspace-execution, not independently on main and not through a parallel remote-routing mechanism. Its existing effort is efforts/client-side-tool-execution. The branch's current HEAD and worktree state must be verified before implementation: earlier Git-tool inspection failed, so the historical SHA is not a verified current base.

The execution-location effort retains ownership of host orchestration, authenticated executor routing, conversation binding, workspace consent, operation approval, identity, duplicate suppression, and reconnect reconciliation. Its unfinished security and execution prerequisites are dependencies, not assumed functionality. The implementation plan must identify which prerequisites block computer use and extend the existing contract rather than duplicate them.

This effort narrowly extends the existing no-persistent-client-service constraint: an on-demand desktop broker may outlive the client that launched it and serve other authenticated clients. This does not introduce a general persistent workspace executor or guarantee workspace-command survival after client exit.

## Goals

Deliver an independent desktop-broker component and shared package dependency used by the CLI and Avalonia. Use gRPC and Protocol Buffers for the Cercano-facing contract, with generated Go and C# bindings. Keep UI concerns and Cua-specific behavior outside that contract.

Require working, verified macOS and Windows integration, including installation, permissions, concurrent-client behavior, and upgrades. Linux is follow-up work; maintain portable interfaces and explicitly reject unsupported environments without fallback.

Support window discovery, accessibility observations, screenshots and capture metadata, reviewed accessibility/pixel actions, keyboard input, explicit foreground control, and truthful action outcomes. Expose reviewed operations rather than automatically forwarding every Cua tool.

Provide the integration surface and a small .NET consumer sufficient to verify Avalonia compatibility. Building the full Avalonia application is not required for this effort.

## Component and routing boundaries

The broker is independent of either client and shared per operating-system user and interactive desktop session. It runs without administrator privileges. The selected executor reaches its local broker through authenticated local gRPC; the existing execution-location connection remains the remote transport. Do not expose a second remote computer-control listener.

The broker owns desktop authorization enforcement, action coordination, observations, operation records, and private Cua-helper supervision. It does not own model calls, conversation history, host credentials, general workspace execution, or a second agent loop.

A replaceable backend adapter translates Cercano requests and outcomes to and from Cua. Clients must not depend on Cua command syntax, tool names, or result formats. Cua can retain its existing private local protocol behind this adapter.

## Shared lifecycle and coordination

The first requester may launch the broker; startup coordination must prevent duplicate brokers for a desktop session. Later clients discover, authenticate, negotiate compatibility, and attach. The launching client has no special lifetime ownership. Closing it must not terminate another client's authorized work.

The broker starts Cua on demand and manages its private endpoint and shutdown. Idle shutdown must respect attached clients, active operations, and unresolved outcomes. Neither component needs login-time startup by default.

Desktop control requires bounded ownership of action sequences, not only serialization of individual calls. A control lease is attributable to the requesting conversation and executor; competing requests wait or receive an explicit busy result. Shared observations must not expose another conversation's private results. Leases coordinate Cercano clients, not human input; do not claim focus-race elimination.

Network disconnection, client detachment, cancellation, broker exit, and helper failure are distinct events. Retain bounded operation records for acknowledgement and reconciliation. Do not replay side effects after a lost response, fabricate success, or report cancellation as proof that delivered input stopped. Crash recovery must surface unknown/interrupted outcomes where certainty is unavailable. Exact retention bounds and recovery mechanics require implementation design and tests, not an exactly-once side-effect promise.

## Installation and permissions

Distribute the broker as a shared, independently versioned package using the existing dependency model. CLI and Avalonia declare dependencies on it; users should not manually install Cua or configure an MCP server. Locate and reuse existing packaging conventions before introducing new machinery.

The package includes a pinned private Cua helper and required notices. Cercano controls upgrades; independent Cua update checks and telemetry must be disabled and verified. Removing either client must not remove a dependency still required by the other. Active users must not be disrupted by an uncoordinated upgrade or incompatible client startup.

Validate signed distribution, stable identity, clean installation, repair, upgrade, and uninstall on both required platforms. macOS Accessibility and Screen Recording grants must be checked under the actual shipped launch path. Do not assume a bundle-id argument assigns permissions, that re-signing preserves notarization, or that permission grants survive updates. Windows desktop-session and privilege-boundary behavior requires equivalent real-platform verification. Do not bypass operating-system security boundaries.

## Security and privacy

Local connectivity alone grants no desktop authority. Protect endpoints and authenticate attachments; bind requests and outcomes to client identity, executor, conversation, desktop session, and operation identity. Reject cross-client substitution, stale identities, unauthorized observations, and incompatible contracts.

Desktop consent is separate from workspace consent and per-operation approval. Screen observation, background input, foreground control, and any clipboard or automation features need explicit permission semantics. Existing permission modes must not accidentally broaden desktop access. UI presentations may differ between CLI and Avalonia, but the underlying authorization contract must be shared.

Provide visible active-control status and a stop mechanism. Before input, validate the target and relevant capture metadata. Unsupported or unavailable executors and revoked permissions fail explicitly; never switch machines silently. Cancellation and revocation cannot undo actions already delivered.

Screenshots may contain sensitive content. Preserve capture identity, dimensions, scale, and coordinate frame across transport; apply bounded payload/retention policies and avoid sensitive diagnostic logging. Clearly disclose and honor the configured vision-processing destination: a local broker does not imply local inference.

The helper is fault isolation and a replaceability boundary, not a security sandbox. Release-artifact licensing, including optional components, must be verified before shipping. Do not include optional perception extensions by assumption.

## Protocol evolution

Use a language-neutral gRPC contract with typed observations/actions, capability and readiness discovery, attributable operation IDs, outcomes, cancellation requests, and reconciliation. Distinguish request acceptance, input delivery, and verified effect.

Initial development uses one explicitly unstable protocol without manual version increments. Rebuild broker and generated clients together. A generated contract fingerprint detects stale definitions; build identity and supported capabilities aid diagnostics. A matching fingerprint does not prove semantic compatibility.

Reject incompatible development attachments clearly. Never kill another client's active broker to satisfy a mismatched development build. Package versions are independent of protocol versions. Before independently released client versions must coexist, establish stable protocol version 1, supported compatibility ranges, and negotiation tests.

## Decisions

### Private Cua helper rather than native embedding

Chosen: private managed helper, behind a Cercano-owned adapter. It offers fault isolation and easier backend replacement while retaining responsibility for endpoint protection, supervision, and signing.

| Axis | Private helper — chosen | Native library |
|---|---|---|
| Integration | Managed local protocol and process | Native bindings in executor |
| Risk | Endpoint and lifecycle handling | Native failures share executor process |
| Outcome | Replaceable backend | Tighter integration |
| Cost | Packaging and supervision | Platform-specific binding/build work |

### Shared desktop-session broker

Chosen: shared broker rather than per-client components. Near-term CLI and Avalonia coexistence warrants central coordination now.

| Axis | Shared broker — chosen | Client-owned components |
|---|---|---|
| Coordination | One desktop authority | Separate components coordinate leases |
| Lifecycle | Multi-client attachment and idle shutdown | Per-client supervision |
| Risk | Shared failure and compatibility boundary | Distributed ownership and competing helpers |
| Outcome | Common foundation for both clients | Smaller initial change |

### Shared package dependency

Chosen: independently versioned shared package, following existing dependency conventions. Neither client exclusively owns installation or updates.

| Axis | Shared package — chosen | Per-client bundled copies |
|---|---|---|
| Installation | Existing dependency machinery | Self-contained client distributions |
| Updates | Shared compatibility-aware lifecycle | Runtime selection among copies |
| Risk | Coordinated upgrade/uninstall required | Path, identity, and version ambiguity |
| Outcome | Stable shared dependency | Duplicated installed components |

### Platform and protocol scope

Chosen: macOS and Windows acceptance, Linux follow-up. Chosen: gRPC shared contract, unstable development iteration without manual protocol bumps, followed by explicit stable compatibility before independent client releases. These decisions must not be silently expanded or weakened during execution.

## Acceptance evidence

Demonstrate explicit client-versus-host targeting with different machines or isolated controlled endpoints and prove no host fallback. Exercise CLI and a .NET client concurrently against one broker: startup races, lease contention, isolated grants/results, first-launcher exit, and orderly shutdown.

On macOS and Windows, use disposable surfaces to verify accessibility input, genuine pixel delivery, keyboard/dialog interaction, capture validation, and foreground control with honest limitations. Verify resulting application state or artifacts independently rather than accepting driver success as proof.

Test unauthorized attachment, stale identity, cross-conversation substitution, denied/revoked permissions, cancellation, lost responses/acknowledgements, disconnect/reconnect, helper crashes, broker loss, mismatched development builds, and supported upgrade paths. No duplicate side effects or invented completion is acceptable.

Verify end-to-end screenshot transport and vision routing through the selected executor, not merely direct Cua MCP image output. Verify signed clean-machine installation and dependency upgrade/uninstall behavior on both required platforms. Test Linux unsupported behavior explicitly.

## Non-goals and limitations

No full Avalonia application, Linux production support, container provisioning, workspace synchronization, automatic execution handoff, machine-wide privileged broker, public Cua endpoint, or guarantee against concurrent human focus changes. No additional software installations, operating-system permission changes, or uncontrolled desktop trials are authorized by this draft.

Precise package/module placement, local transport credentials, retention limits, and platform signing mechanics must be established through bounded source inventory and security review. Material architecture forks discovered there return for approval rather than being hidden as implementation details.
