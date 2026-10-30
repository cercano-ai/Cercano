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

Responses uses direct `http.Client.Do`, not a provider SDK. Its application temperature retry reenters `do`. Coordinated clients use a dedicated HTTP/1 transport (HTTP/2 disabled), keep-alives disabled, and a nonempty POST body without an idempotency header. The innermost transport clone removes `GetBody`. HTTP/1 retry prevention relies on the combined constraints above, not on `GetBody` removal alone. The model-policy layer retains its body reader **before** that stripping; policy is not weakened. No custom transport injection is exposed by production Config. Redirect handling returns the first response without following it (managed model policy may reject it instead). Thus redirects do not inherit permission to perform another credential-bearing attempt.

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

## c914f66b273f bounded regression follow-up — 2026-10-28

Added only test-local fixtures/injection in `internal/llm/responses/admission_regression_test.go`:

- `TestCoordinatedSnapshotMutationRefusesHTTP/{rotation,logout}` pauses after the real coordinated atomic snapshot returns through the readonly Service facade. An independent coordinator sharing a memory record backend and temporary exclusion root commits rotation/logout before admission resumes. Both require `ErrStale` and exactly zero fake HTTP calls.
- `TestCoordinatedAdmittedAttemptOutlivesLogout` pauses after real admission, commits independent-owner logout before allowing fake HTTP transmission, then verifies the invalidated snapshot while fake HTTP is still pending. The admitted payload transmits and the request completes successfully. Channel barriers, not sleeps, establish both orderings and demonstrate that credential exclusion is not held across the admission-to-send gap or HTTP lifetime.
- `TestCoordinatedTemperatureFallbackReadmits/{Chat,StreamChat}` returns a fake temperature-rejection response and logs out the fake admission authority. Both require two snapshot calls and two admission checks, only one successful admission/HTTP call, and the logout error on fallback. This specifically fences the second constructed POST; it is not stream-content or higher-level resilience qualification.
- `TestCoordinatedPolicyAndWireComposition` composes model policy -> replay stripping -> fake wire, retaining the constructed client's redirect callback. A forbidden model is denied before wire access; an approved model reaches wire with its original body and no `GetBody`; its fake 307 is denied with no redirect follow-up. Authority assertions check provider, endpoint, placement and model. Existing standalone redirect and replay-hook tests remain in the focused selection.

Reviewer source audit, checked against the actual local `go version go1.26.3 darwin/arm64` standard library: `net/http/transport.go:818-866` (`shouldRetryRequest`) has an HTTP/2 no-cached-connection retry branch before the HTTP/1 fresh-connection guard. `transport.go:471` recognizes the empty `TLSNextProto` map as disabling HTTP/2. Disabled keep-alives exclude pooled/reused connections (`transport.go:1063,1175`); fresh HTTP/1 connections are refused retry at `transport.go:832`. The nothing-written branch at `transport.go:842-846` considers outgoing length and `GetBody`; later branches depend on `Request.isReplayable` (`request.go:1534-1548`). `rewindBody` (`transport.go:795-813`) can return an unread/unclosed body unchanged even without `GetBody`. Therefore stripping `GetBody` alone is insufficient: the reviewed path relies on HTTP/2 disabled, keep-alives disabled/no reused connections, a nonempty POST without idempotency headers, and stripping **after** model-policy inspection. These are source-audit conclusions, not simulated connection-failure results.

Focused verification (environment `CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off`, working directory `source/server`):

```sh
go test -race ./internal/llm/responses ./internal/hostsvc/credentials ./internal/cloudfactory -run 'Test(Coordinated(SnapshotMutation|AdmittedAttempt|TemperatureFallback|PolicyAndWire|AttemptLogout|ReplayRemovedAtWire|RedirectRefused|UnsupportedBeforeConstructor)|AttemptReadonlySnapshotLogout)' -count=10 -timeout=120s
```

Passed. Initial new-test run caught a test expectation using the configured model rather than the request model for the temperature-cache key; the fixture now supplies an explicit request model. No production issue or production change was needed. No networking, native stores, children, spec/plan/activation/adoption changes, full-suite run, TLS/HTTP2 runtime qualification, or crash/restart qualification was performed. Explicit enabled credential scopes contain only memory/fake dependencies and temporary lock directories.
