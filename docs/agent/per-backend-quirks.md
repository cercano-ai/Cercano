# OpenAI-compatible backend quirks

Current implementation: `source/server/internal/llm/openai/quirks.go` and
`transport.go`. The earlier implementation plan is historical; transport-level
retry policy described there has been removed.

## Backend selection

`CloudProfile.backend` selects adapter quirks. `cloudfactory.BuildCloudProvider`
also infers `mistral` when backend is empty and the provider is Mistral or the
credential-free endpoint hostname is `api.mistral.ai`. Otherwise an empty or
unknown backend uses the defensive default.

| Backend | Images converted to base64 | Error normalization | Mistral structured content |
| --- | --- | --- | --- |
| `openai` | No | Yes | No |
| `mistral` | Yes | Yes | Yes |
| Other/empty (including Gemini and Groq) | Yes | Yes | No |

These are adapter choices, not a claim that every model supports images. The
cloud factory requires affirmative model vision evidence for Chat Completions
and Responses; a missing capability callback does not grant vision support.

`Quirks` contains `MistralContent`, `ImagesAsBase64`, and `NormalizeErrors`.
There is no `Retry` field. The transport preserves explicit zero temperature,
ensures empty tool results still have content, normalizes error envelopes, and
handles successful Mistral structured content. Mistral signed reasoning is not
implemented; see [Mistral](mistral.md) for adapter limits and dated live results.

## Retries belong above the adapter

`internal/inference/resilience` owns bounded, class-driven, narrated retries:
busy gets one same-provider retry (500 ms default, Retry-After capped at two
seconds), then backup/surface; quota, auth, network and unknown errors can fail
over; invalid requests and cancellation surface. Subscription authentication
recovery and output/tool replay guards impose additional restrictions. Primary
account traversal and Secondary's single backup are described in
[cloud routing](../cloud-routing.md). There is no hidden `httpx.RetryTransport`
or retry loop in `normalizingDoer`.

## Verification boundary

This describes repository code, not fresh external conformance testing. Hosted
endpoint compatibility and availability must be tested separately. Historical
Gemini observations remain in [backend notes](llm-backend-notes.md); they do not
certify today's provider behavior.
