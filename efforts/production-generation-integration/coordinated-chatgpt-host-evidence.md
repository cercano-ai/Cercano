# Bounded coordinated ChatGPT host wiring evidence

`TestCoordinatedChatGPTHostGraph` first failed at host `Rebuild` with
`coordinated credential route unsupported`, before the factory was reached.
The host now allows only Responses with an empty route or the explicit ChatGPT
route when credentials are coordinated. Other subscriptions, flavors, unknown
routes, and Bedrock still refuse before construction.

The regression uses an injected, entirely memory-only coordinated authority,
the actual config credential Service/facade, host rebuild, and real cloud factory
construction. It verifies the same profile identity, revision-bearing request and
subscription capabilities, no raw-token fallback, publication of the host graph,
and logout fencing both the captured revision and a subsequent subscription
snapshot. It does not open native credentials or invoke external transport.

Coverage distinction: this is host graph construction and capability coverage,
not host Chat/Stream transport execution. The Responses client owns a private HTTP
transport with no host injection seam; no new public transport seam or global
transport mutation was added for this bounded fix. Direct client tests separately
cover fake-transport atomic admission, model retries, and no legacy fallback.
Neither those tests nor the host graph test establish external transport or
production activation readiness.

Validation (all passed, CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off):

- `go test -race ./internal/hostsvc/providers ./internal/cloudfactory ./internal/llm/responses ./internal/hostsvc/credentials -run 'TestCoordinatedChatGPTHostGraph|TestBuilderCredentialAdmission|TestCoordinatedUnsupportedBeforeConstructor|TestSubscriptionAtomicAdmission|TestSubscriptionModelRetryAdmission|TestSubscriptionNoLegacyFallback|TestSubscriptionAttemptServiceBlocker|TestSubscriptionSnapshot' -count=1`
- `go build -o /dev/null ./cmd/cercano/`

Enabled test scopes use explicit memory/fake injection; no process qualification,
native constructor, live endpoint, or full package suite was executed here.
