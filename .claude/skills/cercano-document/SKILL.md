---
name: cercano-document
description: Generate doc comments for exported Go symbols using routed AI and write them directly to the file. Cercano handles the read-generate-write cycle; the selected model endpoint receives the input.
TRIGGER when: user asks to add doc comments, godoc, or documentation to Go source files.
DO NOT TRIGGER when: user asks to write README/markdown documentation or document non-Go code (do that directly instead).
compatibility: Requires Cercano server running with a usable configured model route. Currently supports Go source files only.
---

# Cercano Document

Generate doc comments for exported Go symbols using routed AI and write them directly to the source file.

## Important: Display the result

MCP tool results may not be visible to the user in the terminal. After calling the tool, you MUST output the full tool result text verbatim in your response so the user can see it.

## Routing and data handling

Saved task routing and redirects determine local or cloud execution. Tool names
do not enforce locality, privacy, cost, or latency. Verify the selected destination
before supplying sensitive input; web research also contacts external search and
page servers. Model output and confidence are not independent verification.

## MCP Tool

**Tool name:** `cercano_document`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| file_path | string | Yes | Path to the Go source file to document. |
| style | string | No | Doc comment style: "minimal" (1-2 sentences, default) or "detailed" (multi-line with params). |
| dry_run | bool | No | If true, report what would be documented without writing changes. |
| project_dir | string | No | Project root directory for context-aware responses. |

## How It Works

1. Parses the Go file using the standard go/ast package
2. Identifies exported symbols (functions, methods, types, interfaces, constants) without doc comments
3. Generates a doc comment for each using the configured model route (one symbol at a time)
4. Inserts comments at the correct positions and formats with gofmt
5. Returns a summary of what was documented

The host agent never sees the file contents — only the summary.

## Safety

- Creates a backup in `.cercano/backups/` before writing
- Validates the result with `go/format`
- Restores from backup if validation fails
- Skips symbols where the model returns garbage

## Examples

**Document a file:**
```json
{"file_path": "internal/engine/ollama.go"}
```

**Preview without writing:**
```json
{"file_path": "internal/engine/ollama.go", "dry_run": true}
```

**Detailed style:**
```json
{"file_path": "internal/engine/ollama.go", "style": "detailed"}
```
