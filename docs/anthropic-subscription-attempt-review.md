# Anthropic subscription attempt integration — scoped verification

Baseline inspected: bfd7d065; Responses ChatGPT SubscriptionSnapshot/admission used as template.

Static fixture review completed before the authorized selector runs. No broad
suite, standalone build, native adapter, activation, adoption, or commit was run.
No physical-send/exactly-once claim.

## Exact offline selection (PASS)

From `source/server`:

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off CERCANO_DISABLE_CREDENTIALS=1 go test ./internal/hostsvc/credentials ./internal/llm/anthropic ./internal/hostsvc/providers -run '^(TestAnthropicSubscriptionSnapshot|TestAnthropicSubscriptionAdmission|TestAnthropicSubscriptionTemperatureRetry|TestCoordinatedAnthropicHostGraph|TestAnthropicEndpointSelection|TestCoordinatedAnthropicHostRefresh)$' -count=1
```

Only these six top-level tests were selected. Every Go test invocation used all
four controls above; fixtures enable credentials only in their isolated scope.

## Safety map

* `TestAnthropicSubscriptionSnapshot`: EnableHermetic; recordFixture memory records
  with temporary file exclusion locks only (no native store); seed uses injected
  store. Explicit Flow.Client RoundTripper and TokenURL `https://issuer.invalid/token`;
  returns JSON directly, never dials. Counting ForbiddenTransport guards default HTTP.
  Fresh/expired, readonly fresh/expired, same-owner logout within issuer callback,
  pending no replay, uncertain issuer/commit, committed snapshot equality and stale
  revision admission are covered.
* `TestAnthropicSubscriptionAdmission`: attemptOwners calls EnableHermetic;
  in-memory records, temporary exclusion locks, no native store. Snapshot source
  panics if legacy Token is used. Snapshot and post-admission callbacks publish
  deterministic logout fences. Counting default forbidden transport; constructor-time
  explicit wire fake. Covers profile/source mismatch, bearer/beta/API-key identity
  and leading Claude Code identity. Post-admission logout allows the admitted
  attempt; pre-admission logout refuses it.
* `TestAnthropicSubscriptionTemperatureRetry`: same isolated memory owners and
  explicit constructor wire; counting forbidden default. Chat fallback resolves
  another snapshot and is fenced after logout. Streaming has no temperature
  fallback in the existing adapter and expects only one snapshot/wire attempt.
* `TestCoordinatedAnthropicHostGraph`: EnableHermetic, secrets.NewMemory plus
  hostCredentialFake capability; real host buildProfile → factory → constructor →
  Chat. Invocation-local Flow with explicit Client/TokenURL; fresh credentials must
  never invoke its counting forbidden issuer transport. Explicit fake wire checks
  api.anthropic.com pinning despite a conflicting profile BaseURL. Counting default
  deny transport. No environment-selected injection.

* `TestAnthropicEndpointSelection`: same memory-only attempt owners and explicit
  constructor wire. Checks subscription config+environment overrides are ignored,
  while direct config and direct SDK environment endpoint behavior are preserved.
* `TestCoordinatedAnthropicHostRefresh`: real host → factory → constructor → Chat,
  real coordinator with an injected memory record backend and temporary exclusion
  locks. Injected Flow, fake issuer, Messages wire, store and builder are installed
  before EnableHermetic enables credentials. Checks expired rotation is committed
  before Messages receives the rotated bearer, endpoint pinning against cfg+env,
  fresh revision, and one issuer call across two Chat calls. Logout during refresh
  and uncertain issuer each block Messages and do not replay the issuer.

All selected scopes clear inherited ANTHROPIC_* SDK environment via EnableHermetic.
Endpoint regression fixtures then deliberately set ANTHROPIC_BASE_URL to
`https://unexpected.invalid`; their explicit fake wires never dial.
No selected fixture uses httptest, a server/socket, child process, native constructor,
or real issuer transport. Existing unselected tests are not approved by this note.

## Observed evidence

1. Minimal reproduction: existing `TestAnthropicSubscriptionAdmission` sets the
   conflicting SDK base URL after EnableHermetic and checks `api.anthropic.com`
   in its explicit BaseTransport. First invocation hit a pre-existing compile
   typo (`routeSubscription`); corrected references to exported `RouteSubscription`.
2. Re-ran only that test before the endpoint fix: **FAIL**, fresh and logout-after
   both observed `unexpected.invalid` instead of `api.anthropic.com`.
3. Constructor now explicitly supplies `option.WithBaseURL` for subscription,
   regardless of config/environment, and stores that canonical endpoint in cfg.
4. Exact six-test selector above: **PASS** in all three packages (credentials,
   Anthropic, providers). Default transport counters remained zero where checked.

Parent verification repeated the exact six-test selector with `-race -count=3 -timeout=120s` and the same four environment controls on 2026-10-28. Exit 0 (4.329s): credentials 1.270s, Anthropic 1.307s, providers 1.603s. No broad-suite results are claimed. Existing unselected socket-based subscription tests were not executed or migrated in this scoped change.
