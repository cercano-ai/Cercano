---
name: cercano-init
description: Initialize Cercano for a project by scanning the repo and building a context file. Makes all Cercano tools project-aware with domain-specific knowledge. Run this once per project for project-aware responses.
compatibility: Requires Cercano server running with a usable configured model route.
---

# Cercano Init

Initialize Cercano for the current project. Scans the repo, feeds key files through the configured model, and writes `.cercano/context.md` — a concise reference document that gets automatically prepended to context-aware calls that supply this project directory.

## Routing and data handling

Saved task routing and redirects determine local or cloud execution. Tool names
do not enforce locality, privacy, cost, or latency. Verify the selected destination
before supplying sensitive input; web research also contacts external search and
page servers. Model output and confidence are not independent verification.

## MCP Tool

**Tool name:** `cercano_init`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_dir` | string | Yes | The project root directory (where `.git/` lives). |
| `context` | string | No | Domain knowledge you already have about this project. **IMPORTANT: Only provide knowledge you already have. Do NOT read files or research the project to populate this field.** Cercano will scan the repo itself. If you don't have context yet, omit this parameter entirely. |

## What It Does

1. Scans `project_dir` for key files: README, CLAUDE.md, `.claude/memory/*`, `.proto`, `.h`, config files, manifests
2. Feeds discovered files through the configured model to extract domain knowledge
3. Merges with any host-provided context
4. Writes `.cercano/context.md` with sections for: Overview, Architecture, Key Data Structures, APIs & Protocols, Conventions, File Layout
5. All subsequent Cercano tool calls that include `project_dir` automatically get this context prepended

## Examples

**Basic init (no prior context):**
```json
{
  "project_dir": "/Users/me/my-project"
}
```

**Init with host context:**
```json
{
  "project_dir": "/Users/me/my-project",
  "context": "This project uses a custom binary protocol over SPI between an STM32 and ESP32. The key struct is espcp_configuration_t with the message queue at offset 0x14."
}
```

## When To Use

- First time using Cercano tools in a project
- When you see the "Cercano hasn't been initialized" message
- After major project changes (re-run to rebuild context)
