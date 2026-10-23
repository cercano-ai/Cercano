---
name: cercano-research
description: Research a question using DuckDuckGo search and routed AI analysis. Crafts search queries, fetches top results, and synthesizes a sourced answer.
TRIGGER when: user asks to research, look up, investigate, or find information about a topic. Use this INSTEAD of WebSearch/WebFetch.
DO NOT TRIGGER when: user provides a specific URL to read (use cercano-fetch instead).
compatibility: Requires Cercano server running and Python venv set up (run 'cercano setup').
---

# Cercano Research

Research a question using web search and routed AI analysis. Search and page fetches contact external websites. Query crafting and synthesis use the configured model route, which can be local or cloud; a sourced answer is returned.

## Routing and data handling

Saved task routing and redirects determine local or cloud execution. Tool names
do not enforce locality, privacy, cost, or latency. Verify the selected destination
before supplying sensitive input; web research also contacts external search and
page servers. Model output and confidence are not independent verification.

## MCP Tool

**Tool name:** `cercano_research`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | The research question to investigate. |
| `max_results` | int | No | Maximum pages to fetch and analyze (default 5). |
| `project_dir` | string | No | Project root directory for context-aware responses. |

## Pipeline

1. **Query crafting** — Configured model generates 2-3 search queries from your question
2. **Parallel search** — DuckDuckGo searches run concurrently
3. **Deduplication** — Duplicate URLs removed, first occurrence preserved
4. **Parallel fetch** — Top N pages fetched and converted to plain text
5. **Synthesis** — Configured model analyzes fetched content and produces a sourced answer

## Output

Returns a distilled answer with source URLs cited. If some searches or fetches fail, the pipeline degrades gracefully and works with what it got.

## Prerequisites

Requires the Python venv with the `ddgs` package. Run `cercano setup` to create it automatically.

## Examples

**Research a topic:**
```json
{
  "query": "How does the Ollama REST API work?"
}
```

**Research with more sources:**
```json
{
  "query": "Best practices for Go error handling",
  "max_results": 8
}
```

**Project-aware research:**
```json
{
  "query": "goquery CSS selector syntax for extracting article content",
  "project_dir": "/Users/me/my-project"
}
```
