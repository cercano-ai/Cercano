# Vision input and image inspection

## Current inspection behavior (source-audited 2026-10-22)

This file is not wholly historical: provider plumbing is implemented and images
now enter through attachments, including inherited child-conversation images.
The original design is preserved below, but its deferred inbound-path statements
are not current limitations. See [CLI image input](../features/cli/image-input/)
and [native inference](../features/deepinfra-infer.md).

The reasoning loop receives stable image-ID placeholders. Use `inspect_image`
with `image_id` and a focused `question`; it is an agent-only tool, not an
external MCP attachment lookup. The vision model receives pixels and the
question in a tool-less leaf request (default 90-second timeout per inspector
call, 512 output tokens). The answer envelope includes image ID, question,
answer and available source/confidence; it is not proof beyond what the image
shows. Missing attachments require reattachment; unavailable vision and provider
failures return explanatory tool text rather than aborting the reasoning turn.

### Routing and fallback

`toolstack.BuildVision` wraps the shared attachment store with a locus-aware
inspector and answer cache. Cloud image capability must be affirmatively known
for the selected model; adapter `SupportsVision` alone only proves transport
support. The cloud profile chain can resolve a confirmed backup without sending
an image to an unconfirmed preferred model.

| Locus | Inspection preference | Fallback |
| --- | --- | --- |
| `cloud_only` | Cloud | No local calls |
| `cloud_primary` | Cloud | Local if configured |
| `open_primary` | Cloud (even though reasoning is local-first) | Local if configured |
| `open_only` | Local | No cloud calls |

Fallback occurs both when the preferred service is unavailable and when its
request fails. Configure a confirmed image model under Cloud → Model tiers, or
local vision for local-only use; if neither permitted lane works, ask for a text
description rather than inventing image contents. The source label reflects the
answering route/model. A local fallback is a separate bounded inspector call,
not a promise that the whole fallback sequence takes at most 90 seconds.

### Cache and attachment limits

Successful answers are cached in memory per conversation, keyed by image ID and
question after trimming, whitespace collapse and lowercasing. Repeating only
casing/spacing changes normally reuses the answer rather than making a fresh
inference. Errors are not cached. Availability and image-presence checks still
pass through to the underlying service. Attachments and answers are not durable;
a resumed session may need the image reattached. Image bytes live in private OS
temporary files, with only metadata/IDs in memory; missing files are lookup
misses. Identical bytes deduplicate within a conversation, and reattachment can
restore a removed temporary file. There are no cumulative conversation image
count/byte caps in this store. Owners close it for cleanup; a crash can leave
files for OS cleanup. This is not durable attachment persistence.

The cache key does **not** include provider, model or locus, and there is no TTL
or concurrent-request coalescing in this wrapper. A routing change alone does
not force a new answer to an identical question; cached source labels describe
the original inspection. `Clear(conversationID)` exists for lifecycle callers,
not as an `inspect_image` argument. No production invocation of this cache
clear method was found in the audit; do not promise eviction on tab dismissal. Do not treat a repeated cached answer as
independent verification.

Source: `source/server/internal/toolstack/vision.go`,
`internal/visioninspect/{visioninspect,locus,cache}.go`, and
`internal/capabilities/builtins/inspect_image.go` (the latter paths relative to
`source/server`). Targeted tests and limitations are recorded in the
[audit report](../audit-agent-specialists.md). No live image inference was run.

## Historical provider-layer design — 2026-06-27

The remaining text records the original implementation scope, not current
capability or URL-fetch security guarantees.

**Status:** Provider-layer plumbing implemented 2026-06-27. Inbound path still deferred. Built together with
[cloud-openai.md](./cloud-openai.md) (the OpenAI provider's adapter consumes the
image block).

Add image content to the `llm` message model and teach every provider adapter to
translate it, so the provider layer is vision-ready and consistent. This is layers
1–2 of image support; the **inbound path** (CLI attach + proto image field — how
images actually enter the agent) is deferred to the broader "tackle images"
initiative.

## 1. Image block (the foundation)

Extend `internal/llm/messages.go` `Block`:

```go
const BlockImage BlockType = "image"

// added to Block (exactly one of ImageData / ImageURL is set):
MediaType string `json:"media_type,omitempty"` // e.g. "image/png" (required for base64; optional hint for URL)
ImageData string `json:"image_data,omitempty"` // base64-encoded image bytes
ImageURL  string `json:"image_url,omitempty"`  // http(s) image URL
```

**Both forms are supported** — base64 bytes *and* an http(s) URL — because the
cloud providers accept either natively. **Exactly one** of `ImageData` / `ImageURL`
is set per block. The asymmetry to design around: **Ollama (local) takes raw bytes
only — no URLs** — so a URL image bound for Ollama is fetched server-side and
base64-encoded before sending (see §2).

An image rides inside a normal message: a user `llm.Message` may carry
`Blocks: [BlockText, BlockImage]` (text + one or more images).

## 2. Adapter translation

Each provider adapter gains image handling and flips
`Capabilities().SupportsVision` to `true`.

- **Anthropic** (`internal/llm/anthropic/adapter.go`) — both native:
  `ImageData` → `{type:"image", source:{type:"base64", media_type, data}}`;
  `ImageURL` → `{type:"image", source:{type:"url", url}}`.
- **OpenAI** (`internal/llm/openai/adapter.go`, new in the sibling sub-project) —
  multi-part content with an `image_url` part: `ImageURL` → the URL directly;
  `ImageData` → a data URI `data:<MediaType>;base64,<ImageData>`.
- **Ollama** (`internal/llm/ollama/adapter.go`) — bytes only. `ImageData` →
  `base64.StdEncoding.DecodeString` → message `Images`. `ImageURL` → **fetch it
  server-side** (HTTP GET, with a context timeout and a sane size cap) → bytes →
  `Images`. A shared helper (e.g. `llm.ResolveImageBytes(ctx, block)`) does the
  base64-decode-or-fetch so the logic isn't Ollama-specific.

A message with mixed text + image blocks must produce the provider's correct
multi-part user message (text and image together), not two separate messages.

**Note on URL fetching:** the Ollama fetch issues an outbound GET to a
caller-supplied URL. With no inbound path yet, no untrusted URL reaches it, but
when the inbound path lands, that fetch is an SSRF surface — bound it (timeout,
max size, and ideally an http(s)-only / no-internal-address check) at that time.

## 3. Capabilities

`anthropic`, `ollama`, and `openai` set `SupportsVision: true`. (Nothing in the
tool-loop branches on it yet; it becomes meaningful when the inbound path lands,
and lets a future router avoid sending images to a non-vision backend.)

## 4. Tool-loop / message flow

No tool-loop change expected — it passes `llm.Block`s through opaquely. The image
block flows through `ConvHistory` / message construction like any other block.
(Confirm during implementation that no block-type switch silently drops unknown
types; if one does, add the `BlockImage` case.)

## Testing

- **messages**: `BlockImage` round-trips through any JSON marshal/unmarshal in the
  `llm` package.
- **anthropic adapter**: text + base64 image → base64 source; text + URL image →
  url source; right media type; `SupportsVision` true.
- **ollama adapter**: base64 image → `Images` with decoded bytes; URL image →
  fetched (against an `httptest` server) → `Images`; `SupportsVision` true.
- **`ResolveImageBytes`**: base64 path decodes; URL path GETs (mock server);
  non-base64 `ImageData` and a failed/oversized fetch surface clear errors.
- **openai adapter**: covered in the OpenAI sub-project — text + image (URL → URL
  part; base64 → `data:` URI part); `SupportsVision` true.

## Out of scope

- **Inbound path** — CLI/clients attaching images, and a proto/request image
  field. Nothing produces image blocks yet; this plumbing is exercised only by
  adapter tests until that lands. (The full SSRF hardening of the URL fetch also
  lands with the inbound path — see §2 note.)
- Image *outputs* / generation; document/PDF inputs.
