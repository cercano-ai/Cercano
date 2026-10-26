# Mistral cloud accounts

Mistral is available in **Cloud Providers** using API-key authentication and
`https://api.mistral.ai/v1`. It uses the existing chat-completions adapter and
cloud-profile secrets store. This cloud provider is separate from the local
`mistral.rs` runtime.

## Setup and model selection

1. Select Mistral in Cloud Providers and create a named account.
2. Enter its API key in the masked key field and save the account. Do not paste
   credentials into chat. Save the account/key before choosing model overrides.
3. Choose models within that account's provider settings and save. The list is
   fetched from the authenticated `/models` endpoint. Newly selected models are
   checked against that account again before saving; image models require
   affirmative vision evidence. Session model overrides also recheck access.
4. Add another Mistral account if needed. Selection never chooses another account
   merely because it can access the requested model.

Mistral discovery does not publish token prices. The UI displays **pricing
unknown**, not zero, and no price-based quality recommendations are invented.
Choose explicit economy/standard/premium overrides for the qualities you use.

## Shared catalog, separate account evidence

**Show Mistral catalog across accounts** opens a read-only view within Mistral's
provider settings. It shows each model once, with account-specific context and
capability information underneath. This explicit combined view can include
private/fine-tuned model names owned by any configured Mistral account. A failure
for one account is shown rather than presenting a partial refresh as complete.

The model dropdown used to configure an account remains filtered to that
account. Different context or vision capabilities reported for the same ID are
not merged into one universal answer. Runtime metadata and delegated-worker
snapshots retain account identity. The Local Models browser is unchanged;
Mistral cloud models are not downloadable entries.

IDs, names, aliases, context limits, capability presence, and retirement dates
are preserved where supplied by the API. Unknown capabilities remain unknown.
Retired models and models without affirmative chat/tool capabilities are not
new agent-model choices. Existing pinned IDs are retained rather than silently
replaced; new selections fail with a diagnostic when access cannot be verified.

Successful discovery is cached for 15 minutes per account configuration; failed
refreshes back off for five seconds and remove that account's available observations.
Changing a key or endpoint, removing an account, or recreating its name invalidates
its observations. An old in-flight response cannot restore obsolete access.

## Verification status

The live observations below are historical October 2026 results (the smoke
test is dated 2026-10-16), not a fresh provider certification. The 2026-10-22
audit ran only deterministic tests; see [audit evidence](../audit-agent-specialists.md).

The provider catalog label remains **untested** pending the remaining live
verification gates, including vision. Live catalog, streamed chat and tool
continuation have passed for `mistral-small-latest`. Automated coverage includes authenticated discovery,
account isolation, sparse metadata and lifecycle fields, redirects and response
limits, credential changes, concurrency, client/server metadata mapping, worker
account identity, and streamed text/tool-result continuation through the factory.

A user-authorized live smoke test on 2026-10-16 initially encountered HTTP 429,
provider code 1300 (`Rate limit exceeded`). Diagnostic requests showed a zero
requests-per-minute allowance despite purchased credits. After the user enabled
a pay-as-you-go subscription, the same production-adapter smoke test passed:

- Authenticated discovery parsed 54 models.
- `BuildCloudProvider` selected `mistral-small-latest` and streamed the expected
  reply (20 input tokens, 2 output tokens).
- A tool call with the expected arguments and its continuation both returned
  HTTP 200, preserving the nine-character tool-call ID. No real tools executed.

The live test used the worktree's production discovery, factory and adapter code,
not a standalone HTTP implementation. Live vision and reasoning remain unverified.

A further live source-analysis test (`TestMistralLiveCodeRead`, opt-in with
`CERCANO_MISTRAL_LIVE_CODE=1` and `MISTRAL_API_KEY`) passed with
`mistral-small-latest`. Through the production streaming adapter, the model
requested a restricted `Read` operation. The test harness read the actual
`internal/llm/openai/mistral_content.go` file and returned it with the model's
exact tool-call ID. The model correctly identified rejection of signed reasoning,
per-line stream buffering, and the 8,388,608-byte response limit. Both inference
requests returned HTTP 200. This exercises a real file read in a bounded harness,
not the full CLI permission/tool-execution loop.

The model wrapped its correct JSON answer in a Markdown fence despite the prompt
requesting JSON only. The test initially rejected that formatting; it now accepts
one exact JSON fence and still checks all three answer values. This result does
not establish strict JSON-output compliance. Each run makes at most two inference
requests and allows reading only the fixed source file; no arbitrary model-selected
path or command is executed.

The same live source-reading test also passed with catalog-confirmed exact IDs
`mistral-large-4` and `zai-glm-5-3`, selected using
`CERCANO_MISTRAL_LIVE_MODEL`. Large 4 passed with a 256-token per-request cap.
GLM 5.3's initial continuation hit that cap before completing its answer; a
bounded retry with a 1,024-token cap passed, with a Markdown-fenced JSON answer.
Both models completed streamed tool calls and correctly analyzed all three code
facts. This does not establish signed-reasoning replay or broad model reliability.
The test now caps each inference request at 1,024 output tokens and refuses a
requested model absent from the eligible live catalog rather than substituting.

The live catalog also confirms `mistral-ocr-4-1`, but OCR has not yet been tested.
Cercano supports attachment-byte retrieval through `ExportImage`, but the live
RPC probe returned not found for an image still accessible to `inspect_image`.
Attachment-tool discoverability and the possible host/worker store mismatch are
tracked in [issue #60](https://github.com/cercano-ai/Cercano/issues/60). The OCR
request was not sent; this is not evidence of a Mistral OCR failure.

The opt-in `TestMistralLiveAdapterSmoke` test requires both
`CERCANO_MISTRAL_LIVE_SMOKE=1` and `MISTRAL_API_KEY` in its environment. It makes
one discovery request and at most three bounded inference requests, stopping on
the first failure. Normal test runs skip it. It does not persist the credential.

Documented structured text and unsigned thinking chunks are normalized by a
Mistral-only adapter. **Signed reasoning and unsupported content chunk types are
rejected explicitly**; signed reasoning replay is not implemented or claimed.
The local fixtures do not establish compatibility with every Magistral model.
