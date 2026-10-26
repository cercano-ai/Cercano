---
name: cercano-explain
description: Explain code or text using the configured model route via Cercano. Returns a clear explanation of what the code does, its key interfaces, and data flow. Use this to understand unfamiliar code, complex algorithms, or dense documentation before deciding what context to include in the host conversation.
compatibility: Requires Cercano server running with a usable configured model route.
---

# Cercano Explain

Explain code or text using the configured model route.

## Routing and data handling

Saved task routing and redirects determine local or cloud execution. Tool names
do not enforce locality, privacy, cost, or latency. Verify the selected destination
before supplying sensitive input; web research also contacts external search and
page servers. Model output and confidence are not independent verification.

## MCP Tool

**Tool name:** `cercano_explain`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `text` | string | No* | Code or text to explain. Provide either `text` or `file_path`. |
| `file_path` | string | No* | Path to a file to read and explain. Provide either `text` or `file_path`. |

*One of `text` or `file_path` is required.

## Response

Returns an explanation covering:
- What the code/text does (high-level purpose)
- Key interfaces and components
- Data flow

## Examples

**Explain code inline:**
```json
{
  "text": "func (s *Server) handleLocal(ctx context.Context, request *gomcp.CallToolRequest, args LocalRequest) (*gomcp.CallToolResult, any, error) { ... }"
}
```

**Explain a file:**
```json
{
  "file_path": "/project/internal/agent/router.go"
}
```
