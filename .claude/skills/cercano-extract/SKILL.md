---
name: cercano-extract
description: Extract specific information from text using the configured model route via Cercano. Returns only the relevant sections matching your query. Use this to pull function signatures, error messages, config values, API endpoints, or other targeted information from large text without placing the entire input into the host conversation.
compatibility: Requires Cercano server running with a usable configured model route.
---

# Cercano Extract

Extract targeted information from text using the configured model route.

## Important: Display the result

MCP tool results may not be visible to the user in the terminal. After calling the tool, you MUST output the full tool result text verbatim in your response so the user can see it.

## Routing and data handling

Saved task routing and redirects determine local or cloud execution. Tool names
do not enforce locality, privacy, cost, or latency. Verify the selected destination
before supplying sensitive input; web research also contacts external search and
page servers. Model output and confidence are not independent verification.

## MCP Tool

**Tool name:** `cercano_extract`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `text` | string | Yes | The text to search through and extract information from. |
| `query` | string | Yes | What to find or extract (e.g. `"error messages"`, `"function signatures"`, `"config values"`). |

## Examples

**Extract error messages from logs:**
```json
{
  "text": "[2026-03-20 10:15:32] INFO Starting server...\n[2026-03-20 10:15:33] ERROR Failed to connect to database\n...",
  "query": "error messages"
}
```

**Extract function signatures from code:**
```json
{
  "text": "package main\n\nfunc ProcessRequest(ctx context.Context, req *Request) (*Response, error) { ... }",
  "query": "function signatures with their parameter types"
}
```
