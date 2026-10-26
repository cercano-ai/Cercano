---
name: cercano-summarize
description: Summarize text or files using the configured model route via Cercano with local or cloud execution determined by routing. Supports brief, medium, and detailed summary lengths.
TRIGGER when: user asks to summarize a file, log, diff, document, or large block of text. Use this INSTEAD of reading the full content into cloud context.
DO NOT TRIGGER when: summarizing short text that's already in the conversation context.
compatibility: Requires Cercano server running with a usable configured model route.
---

# Cercano Summarize

Summarize text or files using the configured model route.

## Routing and data handling

Saved task routing and redirects determine local or cloud execution. Tool names
do not enforce locality, privacy, cost, or latency. Verify the selected destination
before supplying sensitive input; web research also contacts external search and
page servers. Model output and confidence are not independent verification.

## MCP Tool

**Tool name:** `cercano_summarize`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `text` | string | No* | Raw text to summarize. Provide either `text` or `file_path`. |
| `file_path` | string | No* | Path to a file to read and summarize. Provide either `text` or `file_path`. |
| `max_length` | string | No | Target summary length: `"brief"` (1-2 sentences), `"medium"` (1 paragraph, default), or `"detailed"` (multiple paragraphs). |

*One of `text` or `file_path` is required.

## Examples

**Summarize text:**
```json
{
  "text": "The quick brown fox... [long text here]",
  "max_length": "brief"
}
```

**Summarize a file:**
```json
{
  "file_path": "/project/docs/architecture.md",
  "max_length": "detailed"
}
```
