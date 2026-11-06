# Failed-launch ownership: receipt implementation in main

Status (2026-11-03): **receipt implementation is in main**, through `681cfde9` and the receipt-consumption follow-up `d7cbdeb2`. The original isolated verification below is historical, not a current full-suite signoff. The rolling-readiness follow-up is a separate review branch and is not landed; see [rolling readiness verification](rolling-readiness-verification.md). The reproduction and decision discussion below describe the original checkpoint, not current implementation status.

## Implementation and isolated verification

New launches journal an operation before acquisition. Host receipt transitions bind operation, target, conversation and exact epoch; observation/delivery consumes compensation authority under the lifecycle mutex. Only pending, definitively failed launches can abort. Ambiguous acquisition/output retains ownership and an explicit same-operation recovery command. Direct Go output claims delivery before exposing details; Python handoff leaves delivery to CLI acknowledgment and compensates synchronous pre-exec failures. Artifact rollback is operation-bound under the publication lock. Existing-session and disconnect lifecycle semantics are unchanged.

Converted the original leak assertions to compensation regressions. Added fake-host/Python exec-failure and CLI delivery-boundary coverage, abort/attachment races, durable duplicate abort, failed cleanup retry, exact epoch fencing (including later ordinary reacquisition), observation/detach retention, existing-conversation rejection and ambiguous acquisition/output cases.

Recorded by the original implementation worktree (historical):

- Server: `go test -race ./internal/server -run 'LaunchReceipt|LifecycleControl|Retained' -count=1`
- Server: `go test ./internal/server ./internal/broker ./pkg/agentclient -count=1`
- Server: `go test -race ./cmd/cercano -run DevelopmentLaunch -count=1`
- CLI: `go test -race ./internal/ui -run 'DevHost|LaunchDelivery' -count=1`
- Both affected Go executable builds, writing only temporary binaries.
- Python compatibility (3), progress (9), arguments (4), feedback (5), safety (22) tests.

Limits: scoped isolated gates, not full repository/system acceptance. The previously reported PTY baseline was not rerun or independently established by these gates. Python exec-failure integration stubs generation validation to isolate handoff; generation safety is covered separately by launcher safety fixtures. Those isolated checks did not exercise a production runtime or deployment. The implementation subsequently entered main as noted above.

## Original checkpoint (historical)

At this checkpoint the issue was **not fixed**; it contained fixture characterization only.

## Reproduction

An authenticated fake gRPC host records the exact conversation/host/epoch acquired by real Go development-launch subprocesses. Two injected filesystem failures demonstrate that ownership remains retained and no release RPC occurs:

- Exclusive selection creation collision, injected by the fake host after acquisition.
- Current-target rename blocked by a fixture directory, after successful selection creation.

The fixture implements exact-target/exact-owner release so an attempted cleanup would be observable. All state, tokens, database, listener, binaries and artifacts are temporary; no production dependencies or runtime are constructed.

Commands, both passing on 2026-11-03:

```
go test ./cmd/cercano -run '^TestDevelopmentLaunchAuthenticatedConcurrentReuse$' -count=1 -v
go test ./cmd/cercano -run DevelopmentLaunch -count=1
```

These tests **assert the existing leak**, not correct behavior. Convert their expectations when implementing the compensation. This is not Python/CLI handoff verification or full-suite signoff.

## Why a release-on-error patch cannot satisfy the requested safety contract

- `development_launch.go:362-398` acquires before selection creation, publication and stdout delivery. The acquire request contains only a fresh conversation ID; there is no launch-operation receipt or pending-delivery marker. A failed acquire response currently enters host replacement recovery; it must instead fail closed for that operation, without guessing an epoch.
- `internal/server/lifecycle_controls.go:55-74` permits repeat acquire of a ready local capability. It does not establish that a specific launch operation created it or that no observer ever received it.
- `internal/server/lifecycle_controls.go:142-157` distinguishes management release from terminal release, not failed-launch compensation. Management release is not proof of nondelivery.
- `internal/broker/admission.go:216-233,469-495` drops attachment references on detach and allows management closure when current attachments and work are absent. It cannot distinguish never-delivered ownership from a valid previously attached, now detached conversation. Using this as the cleanup predicate would violate the explicit requirement.
- `development_current.go:16-17` deliberately removes conversation and epoch from current-target. Two launches on the same host/generation can therefore publish indistinguishable target contents. Comparing that content cannot identify which launch won.
- `scripts/cercano-launcher.py:554-559` receives a path through subprocess stdout, validates it, then calls execve. There is no separate acknowledged delivery or launch receipt. A subprocess error with partial output does not prove nondelivery. A successful spawn is not successful exec.

## Focused decision

Decision: may the new-conversation launch path add an explicit host-enforced pending/delivered launch receipt, plus operation-bound target publication, rather than treating generic lifecycle release as failed-launch compensation?

Recommended narrow contract:

1. Create a unique launch-operation record before sending acquire, binding selected identity and fresh conversation ID. Store the returned exact host/epoch only after a validated response. A lost/ambiguous response retains that record and reports an actionable unresolved operation; no epoch inference, replacement, or automatic release.
2. Add a launch-specific pending capability at the host. Ordinary attachment/observation permanently consumes its cleanup authority under the same lifecycle lock, including after detach. Explicit delivery likewise consumes it. No timeout, disconnect, UI exit, or mere lack of attachments transitions ownership to releasable.
3. Conditional compensation must authenticate operation + full target identity + conversation/host/epoch, and atomically verify that delivery/observation has not consumed its authority. Use the existing release/persistence machinery after that guard; do not broaden generic release semantics. Keep a bounded-by-explicit-completion operation result for duplicate cleanup and unresolved release retries, not an expiry-based ownership policy.
4. Pass the launch receipt through the Go/Python handoff and into the CLI. Compensate only definite pre-delivery failures. Execve raising in Python is definite exec failure; process spawn, EOF, a timeout, or missing attachment is not evidence. A direct Go command needs the same explicit delivery contract, not an assumption that printing a path equals client exec. Partial/lost stdout remains ambiguous and retains recoverable operation information.
5. Associate publication with the operation independently of conversation ownership. Under current-target.lock, cleanup compares the exact publication operation and never deletes an unrelated same-host winner. Remove only generated artifacts proven owned by this operation; preserve the operation record and actionable retry instructions on cleanup/release failure.
6. Explicit existing selections and resumed conversations never receive this new cleanup authority. Healthy/detached sessions retain ownership. No setup/provider/bootstrap/DB-reader/fingerprinting redesign.

Estimated implementation/test cost: approximately 400–700 production lines across host lifecycle/control, wire/client API, development launch/current-target, Python launcher and CLI startup; approximately 500–900 fixture/test lines, regenerated protobuf output additional, no new external dependency. Main risk is an incorrect observation/delivery race releasing valid ownership; host-lock race tests and real Python exec-failure fixtures are required. Outcome is exact idempotent compensation without a blanket release policy. Side effects are additive launch metadata and a launch-specific protocol; generic disconnect/release behavior stays unchanged.

A file-only cleanup flag or generic LifecycleRelease call is not an acceptable competing design: existing readers/attachments do not atomically consume such a flag, and management release cannot prove never-observed ownership. A launcher-only subprocess supervisor also does not solve observation/ownership proof. Therefore no options matrix is manufactured here. The strongest argument against the recommendation is protocol scope and persistent retry bookkeeping; that cost is necessary if cleanup remains permitted after selection publication while detached sessions must be protected.

No production implementation was written pending this decision. Required remaining verification includes selection/publication failure with actual compensation, real Python receipt/exec failure, direct Go delivery failure, healthy/existing/resumed/detached retention, observer/epoch/publication races, duplicate cleanup, release failure/retry, ambiguous acquire, affected builds, and relevant Go/Python suites (with PTY baseline reported separately if encountered).
