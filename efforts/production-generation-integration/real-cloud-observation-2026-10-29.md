# Real-cloud bounded run: observed credential refusal

> **2026-10-29 later status:** A separate [bounded real-cloud A/B result](real-cloud-ab-result-2026-10-29.md) records successful A/B replies, ordinary resume/history transfer at quiescence, A retirement and a subsequent B reply (cumulative 9 attempts, 83,317 input / 104 output tokens). The original blocked evidence below is preserved and applies to this earlier run; the later success does not establish how its credential blocker was resolved. Independent-task qualification was skipped; full parity and live migration are not claimed.

## Authorization and implementation

The user approved 40 aggregate inference attempts, 120 seconds per attempt, a 30-minute run excluding compilation, and admission stop thresholds of 300,000 reported input/output tokens or 30,000 output tokens, with acknowledged in-flight overruns. Only `openai-responses` / `gpt-6-astra`, Responses/ChatGPT route; no fallback. No refresh persistence or login authorization was granted.

Checkpoint `ee88af0d5047` adds `realcloudacceptance`-only instrumentation at the Responses HTTP boundary. Normal builds keep their existing HTTP client. A private shared ledger coordinates hosts/workers with the existing file lock and atomic file replacement; each application retry enters the boundary anew. Unknown usage halts further admission. An unresolved attempt blocks further admission once its deadline expires. A future run must not reset a ledger to conceal previous traffic. The default acceptance wire disables connection reuse, HTTP/2 negotiation, redirects and body replay. Custom injected transports are trusted test fixtures, not a universal transport sandbox. Cloud factory refuses other profiles/models/routes in this build. This is run-specific code, not a general budget service.

Local checks (no native credentials or network):

```
CERCANO_DISABLE_CREDENTIALS=1 GOPROXY=off GOSUMDB=off \
 go test -tags realcloudacceptance -race ./internal/llm/responses \
 -run '^TestAcceptance' -count=2 -timeout=60s
```

PASS, exit 0. Final run 10.222 seconds (package 9.544s). Includes 40-attempt limit, token/time/missing-usage limits, concurrent clients, four independent test processes sharing the counter, request deadline propagation, no redirect/replay, route rejection, and partial-stream closure. Deadline propagation/error handling is tested deterministically; this is not a claim that remote generation stops on cancellation.

## Actual application behavior

Private artifact root: `/private/tmp/rc-m6lgc0vh` (0700).

- Built and ad-hoc signed actual server/CLI generation A through `publish-dev-generation.py`, with native cgo enabled and **only** `realcloudacceptance` instrumentation (no synthetic provider tags).
- Authored isolated single-profile cloud-only configuration; no existing user config or conversation history was copied. `prepare-development` copied that authored configuration from the private HOME into private state.
- Launched actual `development-launch` / compiled production development-host, using explicit private configuration, conversation state and short runtime root. Launch succeeded.
- Used the actual CLI under a PTY. Sent `Reply with exactly REAL_CLOUD_A_READY. Do not use tools.`
- The application displayed **`openai-responses login required — openai-responses`** before inference. A reconnect and the ordinary details control displayed **`Reason: missing`** for that profile/provider. No login was approved. The additional user prompt also made no HTTP inference attempt.
- Native credential acquisition was attempted through the ordinary read-only credential service. No credential contents were printed or copied. No application credential write, refresh, sign-in or logout was authorized or performed by this run.
- `missing` is the application's classification, not proof that the native item is absent: the pinned keyring Darwin adapter maps zero query results to not-found before checking other query errors. Do not reinterpret this as an established expired-token/refresh-write requirement.
- Only a user turn was present at the first observation. No assistant model reply, independent task, B publication/inference, transfer/resume, MCP execution, or A/B acceptance was established.

Artifacts: `build-A.log`, `build-env.json`, `run-env.json`, `launch-A.out`, `A-first.pty`, `A-credential-details.pty`, `usage.json`, and `process-cleanup.json`. Artifacts are private; do not commit local bearer token files or arbitrary PTY contents.

## Stop and cleanup

Marked the ledger halted with zero active attempts. Management release and retirement refused with `retained sessions or work prevent transition`; this is a recorded unsuccessful cleanup path, not successful retirement qualification. Sent SIGTERM only to the invocation-owned generation-A development-host PID 21852 after checking exact executable, private state argument and process start identity. Its original identity disappeared. Both test CLI children were explicitly stopped and waited. No installed launcher or preexisting agent/runtime was stopped. Native shared runtime was not used (cloud-only configuration).

## Usage report

Acceptance HTTP inference attempts: **0 / 40**.
Reported acceptance input tokens: **0**. Output tokens: **0**. No acceptance provider reply or usage event exists; zero follows from zero admitted HTTP attempts, not missing usage being treated as zero.

Further delegation this turn: **none**. No audit/review agents were launched. Local controls and builds made no model calls. Main-chat preparation inference is still cloud preparation; its token/billing usage is not exposed by these tools and must be obtained from host/provider accounting rather than estimated or claimed zero. The earlier failed readiness task reported 1,026,039 provider token-volume units including cached input, not a billing total; it is separate from acceptance traffic. Do not report the acceptance ledger as total project spend.

## Blocking input

Observed blocker is application credential acquisition classified **missing**, not a verified refresh requiring persistence. Normal sign-in would write the existing profile's credential and possibly rotate/replace it; it is not authorized by the read-only acceptance permission. Stop and obtain specific user direction before changing that identity. No account/model substitution, automatic sign-in, token inspection/dump, or weakened read-only policy is justified.

The real-cloud A/B journey is **blocked, not passed**. Everyday-use qualification, native credential usability, real model replies, concurrent A/B behavior, settings/MCP real inference, transfer/resume and clean retirement remain pending for this run.
