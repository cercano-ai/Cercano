# Coordinated static Chat Completions — pending fixture review

No tests or builds have been run for this change. Formatting only. Do not run broad package suites: existing OpenAI diagnostic fixtures include socket-based servers.

## SDK evidence

Pinned dependency: github.com/sashabaranov/go-openai v1.41.2. Cached `client.go:130-156` (`sendRequest`) and `174-196` (`sendRequestStream`) each call the configured HTTP client's Do once, with no retry loop. These ranges were read during implementation. The task's prior qualification also establishes value-copied config and no runtime environment credential lookup; those claims were not independently requalified here.

The new outer Do wrapper refuses diagnostic contexts before credential reads, snapshots payload and revision together, admits that revision, and replaces bearer auth on a cloned request. Normalization then model policy remain in their previous order. Only the innermost transport strips GetBody. Production coordinated transport uses fresh HTTP/1 connections, no keepalive and no redirects; nonempty POST and absence of idempotency markers are required. Legacy default transport selection is unchanged.

Factory and host enable only the empty-route static chat lane. Factory refuses subscription sources; Mistral backend derivation and accounting metadata remain intact. Injected transports are trusted constructor-local dependencies and must not implement retries themselves. Logout does not cancel an already admitted request. Higher-level resilience remains outside the single-attempt guarantee. Remote image resolution is outside this HTTP attempt boundary; fixtures contain no images.

## Exact review selectors (not executed)

From `source/server`:

- `./internal/llm/openai -run '^TestCoordinatedChat(MemoryBoundaries|SingleDispatchErrors|DiagnosticRefusal|PolicyBeforeStrip)$'`
- `./internal/hostsvc/providers -run '^TestCoordinatedOpenAIHostGraph$'`
- `./internal/cloudfactory -run '^TestCoordinated(UnsupportedBeforeConstructor|StaticChatConstruction)$'`

Safety map: new OpenAI tests use a wholly in-memory request authority and injected non-dialing transports installed before SDK construction. Default HTTP is counting deny-all. Boundary interleavings are synchronous deterministic hooks (not concurrent stress tests): snapshot/logout denies dispatch; admitted/logout permits the admitted attempt; rotation and retained-client logout are exercised for Chat and Stream. Error cases count one dispatch for transport failure, 401, 429, 500, and 307. Diagnostic refusal counts zero credential reads and wire calls. Policy denial counts zero wire calls. Host graph uses an in-memory store/authority and the real host builder -> factory -> SDK -> fake wire path, then verifies retained-client refusal.

No fixture opens sockets, native stores, or child processes. EnableHermetic additionally clears inherited OPENAI_*, AZURE_OPENAI_* and environment names containing INTEGRATION; ordinary Enable is unchanged.

Limitations for parent review: these tests have not compiled or run; policy test proves denial remains active but does not independently assert every parsed model field; boundary hooks are not a concurrent coordinator stress test; host graph exercises Chat only; factory guard coverage retains the existing unsupported-route table rather than an exhaustive subscription-source matrix. This is not runtime/adoption/activation approval.
