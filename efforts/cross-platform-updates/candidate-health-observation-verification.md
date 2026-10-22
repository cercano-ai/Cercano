# Read-only candidate health observations

Implemented directly following user approval. `Candidate.ObserveHealth` combines an owned-lifetime sample, native listener observation, the existing health RPC, and later listener/lifetime samples. It performs no launch, retry, discovery, signaling, selection change, state write, activation completion, or cleanup.

The result distinguishes unknown, inconsistent, reported-not-ready, and consistent-observations-only. The last status explicitly means peer-not-authenticated: sequential listener observations do not establish exclusive endpoint ownership, bind a TCP connection to a process, or prove future liveness. The PID in the response is compared for consistency only. Attached-client reconnection remains unobserved.

Only explicit numeric loopback endpoints without zones or zero ports are accepted. Network work has a five-second budget capped by the caller deadline; RPC response size is limited to 16 KiB. Synchronous native inspection cannot be preempted by context cancellation, so context is checked between observations and before accepting a result. Unsupported old agents, inspection failures, changed lifetimes, missing listeners, transport errors, and cancellation cannot yield consistent observations. Returned reason slices are copied.

Verification on macOS:
- Full `go test -race ./internal/updatecoord/... -count=1 -timeout=150s` passed.
- Focused health tests passed three race-enabled repetitions.
- Candidate package `go vet` passed.
- Windows/amd64 and Linux/amd64 builds, test compilation, and vet passed; these are not native execution evidence.

Tests cover sample ordering, before/after uncertainty, identity changes, PID/version mismatch, contradictory readiness, response copying, cancellation, invalid requests, nil candidate, actual loopback RPC transport (including Unimplemented, oversized response and deadline), and an exact owned disposable child with native listener inspection and real health RPC. The Windows positive integration fixture uses the existing owned permissive-job harness rather than weakening production breakaway policy.

No live agent or user installation was changed. The new API has no production orchestration caller. No push or CI execution was performed for this slice. At that checkpoint, health-success policy and update completion remained separate gates; the approval below supersedes the policy blocker, not the observation limitations.

## Superseding user approval — 2026-10-22

Exact owned candidate identity/listener, expected release, and readiness within a bounded deadline are now the approved update success gate. Attached terminals reconnect independently and cannot block success. Sequential observations still do not authenticate the TCP peer or promise future liveness. Failed/uncertain health must retain previous-version and recovery information; rollback requires proven owned-candidate exit, never timeout or PID alone. No schema change, live installation/process stop, cleanup, or rollback execution is authorized by this slice.
