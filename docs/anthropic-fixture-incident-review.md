# Anthropic fixture incident — bounded remediation and verification

## Evidence limits

The stopped child reportedly received HTTP 401 from the default api.anthropic.com endpoint while using a synthetic key. This is a child-reported incident, not a packet capture or independently reproduced observation. Exact request count and payload are unverified. There is no proof of native credential-store access. Do not infer either native access or its absence from the 401. No incident reproduction was performed during this remediation.

The reported SDK root/service Options copy explains why changing root Options after construction is not a safe fixture seam. The inspected stopped-task fixtures instead changed Messages.Options, but also rebuilt wrappers after construction. The inspected coordinated messagesHTTPClient explicitly created an http.Transport, bypassing the default transport guard. A default-only trap therefore did not establish hermeticity.

## Static auth-isolation review

Static review found that Enable cleared ANTHROPIC_API_KEY but did not clear the rest of the ANTHROPIC_* namespace. Prior SDK inheritance of alternate auth (including ANTHROPIC_AUTH_TOKEN) was therefore possible; whether any such state existed or was used in the reported incident is uncertain, not established. The reported synthetic API key alone does not establish that no inherited authentication was present.

Enable remains unchanged. EnableHermetic now clears every inherited ANTHROPIC_* variable via t.Setenv, including auth-token and base-URL overrides, with scoped cleanup restoration. The helper regression seeds synthetic ANTHROPIC_AUTH_TOKEN, ANTHROPIC_BASE_URL and an arbitrary namespace override, checks that ordinary Enable preserves them, checks hermetic clearing of the entire namespace, and checks cleanup restores the seeded values. The legacy fake wire now requires empty Authorization and X-Api-Key equal to the synthetic snapshot, without printing either value. Explicit fake injection remains before SDK construction. After the independent review and namespace fix, the parent ran the exact selection below under the race detector twice; all four packages passed. This establishes bounded fixture behavior, not retrospective incident attribution.

## Review map

- `source/server/internal/llm/anthropic/client.go`: invocation-local Config.BaseTransport; SDK receives the completed HTTP client before construction. Unexported newClientWithFallback permits a counting, non-dialing fallback in negative fixtures. Nil production dependencies keep existing defaults.
- `source/server/internal/llm/anthropic/admission.go`: injected wire sits below auth, model policy and coordinated non-replay wrappers. No global production switch.
- `source/server/internal/llm/anthropic/admission_test.go`: all selected SDK clients now use constructor injection, never service/root Options mutation. Legacy retained-key characterization also uses the real wrapper stack. Omitted/misapplied fake negatives substitute ForbiddenTransport for explicit constructor fallback, without sockets. Policy, stale snapshot, admitted attempt, retry, redirect, streaming and replay fixtures retained. HTTP contexts and barrier waits are bounded; gates release through idempotent cleanup.
- `source/server/internal/cloudfactory/factory.go`: invocation-local AnthropicBaseTransport forwarded on coordinated, legacy and subscription Messages constructors.
- `source/server/internal/cloudfactory/admission_test.go`: constructor-only supported routes inject forbidden wire; unsupported routes use panic-on-credential-access authority and reject before constructor.
- `source/server/internal/hostsvc/providers/coordinated_messages_test.go`: memory authority and actual factory graph, with explicit forbidden wire injected by the test builder before SDK construction. No HTTP call is expected here.
- `source/server/internal/credentialtest/isolation.go`, `isolation_test.go`: Enable retains its HEAD loopback-only semantics for existing fixtures. New EnableHermetic layers deny-all default HTTP, including localhost/IPv4/IPv6 loopback, only for the selected Anthropic/factory/provider fixtures. ForbiddenTransport remains a counting trap with no DNS, sockets or TLS. Helper tests verify legacy loopback forwarding through an in-memory transport, hermetic rejection/counting, and cleanup restoration without starting a server.

Native isolation is structural for these selected fixtures: attemptRecords is an in-memory record backend passed explicitly to NewCoordinatedStore (temporary directories are exclusion locks only); hostCredentialFake embeds secrets.NewMemory; cloudfactory requestFake panics on credential access. No native opener or production native-opener environment bypass was introduced. Neither Enable nor EnableHermetic is a sandbox for arbitrary native constructors and must not be used as one. Its environment changes are scoped with t.Setenv and restored; no parallel globals are permitted.

## Parent verification — 2026-10-28

Working directory: `source/server`. Outer credential admission disabled; enabled scopes use the reviewed memory-only dependencies and hermetic helper.

```sh
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off \
 go test -race ./internal/llm/anthropic ./internal/cloudfactory ./internal/hostsvc/providers ./internal/credentialtest -count=2 -timeout=60s -run '^(TestLegacyMessagesRetainedKeyLogoutGap|TestMessagesSnapshotLogoutRace|TestMessagesAdmittedBeforeLogout|TestMessagesRetryAndRedirect|TestMessagesSDKRetriesDisabled|TestMessagesPolicyBeforeReplayRemoval|TestMessagesUnsupportedDirectConfig|TestMessagesTransportConstraints|TestMessagesConstructorIsolation|TestCoordinatedUnsupportedBeforeConstructor|TestCoordinatedStaticMessagesConstruction|TestCoordinatedMessagesHostGraph|TestIsolation|TestAWSInputsAreIsolated|TestEnableLoopbackPolicy|TestHermeticIsolation)$'
```

This is an exact allowlist, not a whole-package safety claim. All other tests are excluded. In particular, existing Anthropic client/stream/auth/accounting/usage/normalize tests use httptest servers; cloudfactory Mistral roundtrip tests use loopback (their existing Enable semantics are unchanged); MistralLiveAdapterSmoke and MistralLiveCodeRead are live tests; BuildBedrockProvider enters a constructor outside this bounded audit; providers reasoning_diagnostic_test.go creates loopback endpoints. Those routes are unsafe for this no-network review and have not been converted or authorized. Full package/all-repository runs remain prohibited pending separate fixture remediation.

Parent command exited 0 in 4.244s: anthropic 1.370s, cloudfactory 1.361s, providers 1.762s, credentialtest 1.308s. No incident reproduction, full-suite execution, native qualification, activation, adoption, installation, push or merge was performed. The legacy characterization deliberately asserts the preexisting retained-key defect; it is not a security-pass assertion. Static Messages is the bounded coordinated path; subscription Messages remains refused. Backend replacement/quarantine guarantees and native activation remain unqualified. These passing tests do not establish whether inherited authentication was present in the earlier outbound request.
