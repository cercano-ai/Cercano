---
name: cercano-classify
description: Classify or triage text using the configured model route via Cercano. Returns a category, confidence level, and brief reasoning. Use this for quick local triage of errors, logs, code quality issues, bug reports, or any content that needs categorization through the configured model route.
compatibility: Requires Cercano server running with a usable configured model route.
---

# Cercano Classify

Classify or triage text into categories using the configured model route.

## Important: Display the result

MCP tool results may not be visible to the user in the terminal. After calling the tool, you MUST output the full tool result text verbatim in your response so the user can see it.

## Routing and data handling

Saved task routing and redirects determine local or cloud execution. Tool names
do not enforce locality, privacy, cost, or latency. Verify the selected destination
before supplying sensitive input; web research also contacts external search and
page servers. Model output and confidence are not independent verification.

## MCP Tool

**Tool name:** `cercano_classify`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `text` | string | Yes | The text to classify or triage. |
| `categories` | string | No | Comma-separated list of categories to choose from. If omitted, the model determines appropriate categories. |

## Response

Returns a structured classification:
- **Category:** The determined category
- **Confidence:** high, medium, or low
- **Reasoning:** One sentence explanation

## Examples

**Classify an error with predefined categories:**
```json
{
  "text": "FATAL: password authentication failed for user \"postgres\"",
  "categories": "auth,network,configuration,data,unknown"
}
```

**Open-ended classification:**
```json
{
  "text": "The API returns 200 but the response body is empty when the user has no profile picture set"
}
```
