# Chat Completions admission — parent verification, 2026-10-28

Static review found the exact new fixtures memory-only with constructor-time non-dialing wire transports and `EnableHermetic`. No broad test selection was approved. Cached sashabaranov/go-openai v1.41.2 source was inspected: Chat and streaming invoke configured Do once and the SDK does not read runtime credentials from environment. Injected custom transports remain trusted and cannot be certified retry-free by the wrapper.

The implementation child's test attempts could not launch its Go executable. Parent verification first caught an accidental undefined test variable, removed it, then observed Chat and Stream 307 tests failing because the SDK considers status below 400 successful (client.go:219–220). Redirects were not followed, but failure was not surfaced. The coordinated doer now closes a 3xx response and returns a non-secret refusal, without changing legacy behavior.

Parent ran from source/server:

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off CERCANO_DISABLE_CREDENTIALS=1 \
 go test -race ./internal/llm/openai ./internal/hostsvc/providers ./internal/cloudfactory \
 -run '^(TestCoordinatedChat(MemoryBoundaries|SingleDispatchErrors|DiagnosticRefusal|PolicyBeforeStrip|PolicyApproved|TransportConstraints|AuthorityFailuresAndClone)|TestCoordinatedOpenAIHostGraph|TestCoordinated(UnsupportedBeforeConstructor|StaticChatConstruction))$' \
 -count=3 -timeout=120s
```

PASS, exit 0 (3.167s): OpenAI 1.344s, providers 1.532s, cloudfactory 1.756s.

Fixtures cover synchronous boundary interleavings, retained client rotation, Chat and Stream refusal/success paths, explicit errors for 401/429/500/307/transport failures with single wire dispatch, diagnostic refusal, policy body inspection before replay stripping, method/body/idempotency refusal, constructor transport constraints, snapshot/admission failures, authorization clone preservation, and real host-to-factory-to-SDK fake-wire requests. This is not simultaneous-process/native-provider qualification. No sockets, native credential opening, paid inference, broad suites, live activation, or adoption were performed. No physical-send, TLS, actual connection-failure, or arbitrary injected-transport retry guarantees are claimed.
