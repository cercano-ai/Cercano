# Bounded credential request-attempt admission — 2026-10-28

## Approved semantics (supersedes physical-send wording in earlier notes)

The explicit user approval makes atomic **request-attempt admission**, not physical transmission, the logout boundary. Successful durable logout replacement prevents subsequent admissions using the invalidated revision. An already admitted attempt may transmit or complete afterward. Admission does not attest that bytes were dispatched, nor cancel requests or revoke remote tokens.

The short credential exclusion covers fresh state/revision validation only. No HTTP operation, issuer, response read, or stream runs under it. Snapshot returns payload and revision from one record read under exclusion; admission rereads and compares active state/revision under the same exclusion used by logout. The gap between snapshot and admission is safe: a changed revision refuses admission. The gap between admission and HTTP is explicitly permitted.

`RequestCredentials` is a caller-owned one-attempt contract: each constructed Responses POST calls it once; no admission result or credential payload is cached on the reusable client. Application retries and follow-ups reconstruct and readmit. It is not an unforgeable general-purpose ticket API: arbitrary callers must not treat a successful call as reusable permission. Service and readonly forwarding preserve policy and coordinated capability checks; readonly admission/snapshot confer no mutation permission.

## Implemented, bounded coverage

- Existing Responses static API-key path through host providers -> cloudfactory -> Responses `do`, streaming and nonstreaming. A retained client holds the facade/profile, not a snapshot. Existing temperature fallback and higher-level retries reenter `do` and obtain fresh snapshot/admission.
- Factory refuses coordinated Messages, Chat Completions, subscription routes, and Bedrock before provider constructors. Host provider service refuses these routes before credential/token-source acquisition. Direct coordinated Responses misuse with token source/nonempty route also refuses.
- Coordinated server model discovery and host reasoning diagnostics explicitly refuse before their authenticated work. No supported discovery/subscription refresh implementation is claimed.
- Existing opt-in coordinator only: native adapters, backend qualification, legacy adoption, production activation and stable shared lock-root qualification remain unchanged/disabled.

## Retry / redirect audit and limits

Responses uses direct `http.Client.Do`, not a provider SDK. Its application temperature retry reenters `do`. Coordinated clients use a dedicated HTTP/1 transport (HTTP/2 disabled), keep-alives disabled, and a nonempty POST body without an idempotency header. The innermost transport clone removes `GetBody`, preventing replay of the body by the standard transport. The model-policy layer retains its body reader **before** that stripping; policy is not weakened. No custom transport injection is exposed by production Config. Redirect handling returns the first response without following it (managed model policy may reject it instead). Thus redirects do not inherit permission to perform another credential-bearing attempt.

Tests inspect replay-hook removal and use a fake 307 transport to establish no follow-up HTTP call. They do not inject real connection failures, exercise TLS/HTTP2, or independently qualify every Go transport internal retry branch; no claim that unobserved SDK attempts were fenced. These constraints intentionally avoid relying on hidden retries receiving fresh admission. Any future transport/replay/header/HTTP2 change needs renewed qualification.

Repository Anthropic client explicitly disables SDK retries, but application fallback and subscription authentication still need admission. OpenAI normalizingDoer and separate reasoning diagnostic dispatch are not integrated; third-party SDK retry internals are not qualified here. AWS default identity and credential-provider traffic remain undefined/unqualified, so coordinated factory construction refuses Bedrock entirely.

## Remaining paths / non-claims

This is a foundation plus one real request path, not system-wide credential completion. Worker credential fetching transfers raw credentials and does not carry an admission capability; worker-local construction/diagnostics, agent/development direct factory calls, directly constructed legacy SDK clients and previously exported raw credentials are not covered by the supported host facade path. Do not activate coordinated credentials across those graphs. Factory callers must explicitly supply the capability; the factory cannot infer credential provenance from a string. No remote revocation or protection against incompatible readers/writers is asserted. Subscription refresh/pending-state integration, supported discovery, native failure/quarantine guarantees and production activation still require separate work. Existing Service subscription acquisition refusal is preserved.

## Verification

All Go commands used `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`, from `source/server`. Only explicit fake/memory dependencies and temporary lock directories were used. No native stores, real providers, live configs/data/processes, enabled children, installers or activation.

- Focused tests passed: `TestAttemptReadonlySnapshotLogout`, `TestCoordinatedAttemptLogout`, `TestCoordinatedReplayRemovedAtWire`, `TestCoordinatedRedirectRefused`, `TestCoordinatedUnsupportedBeforeConstructor`.
- Same selection across credentials/Responses/cloudfactory passed `-race -count=10`.
- Providers/server compiled with `go test ... -run '^$'` (not runtime route qualification).
- Negative control temporarily bypassing Responses admission failed `TestCoordinatedAttemptLogout` with `attempt not admitted`; source restored immediately. This is a regression sensitivity check, not a native/process reproduction.
- Initial compilation exposed missing `fmt` import in provider refusals; corrected and compilation repeated successfully.

No full suite, native qualification, live network, or crash/restart durability run was performed for this slice. Previous process qualification remains limited to its fake-backend contract.
