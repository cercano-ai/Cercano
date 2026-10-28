# Credential request admission audit — 2026-10-28

Read-only child audit task_9f31ae420ae156356bac0c1c92bc5121; source findings, not execution evidence. Its statement that representation approval remained pending is superseded by the user's approval of minimal versioned records at existing keys, with secret-free logout markers. Adoption and activation remain disabled.

## Request-start coverage obligations

Provider creation is not request admission. cloudfactory/factory.go passes cached keys into clients. Retained clients must carry credential identity and revision, and each authenticated request attempt must participate in logout ordering. A revision check followed later by HTTP dispatch leaves a race. Define the request-start handoff explicitly without retaining the short commit exclusion through issuer calls, responses, or streams.

Source leads (relative to source/server):
- internal/llm/responses/client.go:178 HTTP Do; authorize at :199; static keys cached. Token return alone is not dispatch fencing.
- internal/llm/anthropic/client.go:85–93 SDK cached key and injected HTTP client; SDK retries disabled. auth_subscription.go:40 obtains token. Application retries need admission.
- internal/llm/openai/transport.go:53 normalizingDoer.next.Do; client.go:83–84 injection. reasoning_diagnostic.go:300 separate dispatch. SDK retries not fully audited.
- internal/llm/bedrock/client.go:44–80 AWS default credential chain; model HTTP wrapper excludes credential-provider traffic. Converse/ConverseStream :112/:145. Native-profile identity mapping is not established; do not claim AWS credentials covered without it.
- internal/server/cloud_models.go:109–119,184–194 discovery also reads credentials and dispatches authenticated HTTP.
- Redirects and underlying retry attempts need explicit coverage, not assumed fencing from a top-level Chat call.

Subscription sources load/refresh/Save through Store.Set (anthropicauth/source.go and chatgptauth/source.go). Serialize refresh separately, reread after admission, persist uncertainty before issuer work as required by the approved contract, and revalidate publication. Model request admission remains necessary for already-fresh tokens.

## Backend semantics

internal/secrets/secrets.go:89–93 simply forwards keyring Set/Delete. This does not establish atomic replacement, preservation on failure, crash durability, or ambiguity resolution. Native backend source was not inspected in this bounded audit. Isolated replacement-failure tests must distinguish failures before and after mutation; native capability must remain unqualified until established. No native operations authorized.

## Limits

Partial inventory only. No tests, edits to production, native reads, issuer/network calls or process execution in the audit. Fake operation-level Bedrock tests do not test SDK transport retries. No exhaustive cached-client or redirect coverage established. Provider construction tests cannot substitute for logout request admission tests.
