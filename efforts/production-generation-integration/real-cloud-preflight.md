# Real-cloud A/B preflight — 2026-10-29

Status: paused before deployment inference at token-budget permission boundary. No test host launched and no native credential operation performed.

## Confirmed identity

`session_model status` returned effective profile `openai-responses`, model `gpt-6-astra`, override null. A selective YAML parser read only nonsecret routing fields from the ordinary configuration and emitted only the matching profile's allowed fields: name `openai-responses`, flavor `responses`, route `chatgpt`, base_url empty. No config contents/credentials were copied into a fixture. Account selection is the existing service/profile identity; no account email or token was read. Exact model comes from session status, not guessed from a label.

Repository baseline: branch `feat/production-generation-integration`, HEAD `db65ff48ef2d2faef3ac4f13c3baa233613dc31f`; tested `a3acd52a` is ancestor. Existing unstaged plan.md preserved. Baseline inspection was delegated read-only. No push/merge/install/process changes.

## Concrete budget boundary

`source/server/internal/llm/responses/client.go:181–185` explicitly omits `max_output_tokens` for `RouteChatGPT`; comment records backend rejection of that field. `buildRequest` only forwards `req.MaxTokens` on other routes. `NewClient` pins the ChatGPT backend regardless of configured base URL. Do not silently substitute an API-key profile or local model to obtain a server-side output limit.

Proposed test limits, NOT YET IMPLEMENTED OR EXERCISED:

- Maximum 40 HTTP inference attempts aggregated across A/B, workers, continuations, retries and background inference; no fallback provider/model.
- 120-second client-side deadline per attempt; 30-minute overall test window excluding compilation.
- Stop admitting requests at 300,000 cumulative provider-reported input+output tokens or 30,000 output tokens, whichever occurs first. Count cached input conservatively as input where reported. Missing usage must stop the run rather than count as zero.
- Target brief replies; these are instructions, not an enforceable model output limit.

The token thresholds cannot be hard billing limits: usage arrives during/after an attempt, and in-flight work may exceed the threshold. Cancellation/timeouts do not guarantee the remote server stops generation or billing. Request/time enforcement must also be installed and verified before run, not inferred from a tool-loop limit. User permission is required to accept this specific token-overrun exposure before cloud test inference.

## Credential boundary

Use only normal credential service at the selected existing profile, guarded read-only. `developmentCloudFactory` passes a read-only service to ChatGPT source. `internal/chatgptauth/source.go` checks mutation permission before expired-token refresh. No login/logout/replacement/adoption is authorized. Token freshness remains unknown; if normal acquisition reports refresh requiring persistence, pause and request that specific write permission. Do not inspect or dump stored token material for diagnosis.

## Work performed / limits

A narrow readiness subtask exhausted its budget without a completed findings report; its result is not verification of aggregate request enforcement. Direct source inspection established the ChatGPT max-output omission. No acceptance inference, model reply, nine-step journey, native credential qualification, or full regression pass is claimed. Development assistants used normal delegation routing for source inspection; that is separate from test-provider usage. Test-provider usage: zero requests initiated by the deployment harness (not launched).

Next: obtain permission for monitored token thresholds with in-flight overrun; then implement/verify bounded usage admission without changing provider identity or inserting credential-copying proxies, author disposable configuration and fixed MCP barrier definitions, build untagged artifacts, and perform PTY/human terminal acceptance. No live launcher replacement or history migration.
