# Creating Agent Skills for Cercano

This guide covers how to write [Agent Skills](https://agentskills.io) (SKILL.md files) that wrap Cercano's MCP tools. Skills provide discovery instructions for compatible agents; supported locations and activation behavior depend on the host.

## What is a Skill?

A skill is a `SKILL.md` file that tells an agent:
- **What** the tool does (name, description)
- **When** to use it (triggers, use cases)
- **How** to call it (parameters, examples)

Skills are prompt instructions, not tool registrations. When an agent activates a skill, it reads the SKILL.md content into its context and follows the instructions using its existing tools (MCP calls, Bash, file I/O, etc.).

## Directory Layout

```
your-project/
  .agents/
    skills/
      your-skill-name/
        SKILL.md          # Required — the skill definition
        scripts/           # Optional — helper scripts the agent can run
        references/        # Optional — reference docs the agent can read
        assets/            # Optional — templates, configs, etc.
  .claude/
    skills/
      your-skill-name/
        SKILL.md          # Copy for Claude Code discovery (slash commands)
```

- `.agents/skills/` is the shared location used in this repository; check whether your host scans it.
- `.claude/skills/` is Claude Code-specific. Skills here appear as `/your-skill-name` slash commands.
- The directory name must match the `name` field in the SKILL.md frontmatter.

## SKILL.md Format

A SKILL.md file has two parts: YAML frontmatter and a markdown body.

### Frontmatter

```yaml
---
name: your-skill-name          # Required. Lowercase, hyphens only. Must match directory name.
description: >                  # Required. What this skill does and when to use it.
  One or two sentences that help the agent decide whether this skill
  is relevant to the current task. Max 1024 chars.
compatibility: >                # Optional. Environment requirements.
  Requires Cercano server running, MCP connected, and a configured Reconnaissance task route.
license: MIT                    # Optional. License name or path to LICENSE file.
metadata:                       # Optional. Arbitrary key-value pairs.
  author: your-name
  version: "1.0"
---
```

**Key rules:**
- `name` must be lowercase with hyphens (no spaces, underscores, or uppercase). Max 64 chars.
- `description` is what agents use to decide relevance — make it specific and action-oriented.
- The `name` must exactly match the parent directory name.

### Body

The markdown body contains the instructions the agent will follow. Structure it however makes sense, but typically include:

1. **A title and short summary** — what this skill does.
2. **The MCP tool name** — so the agent knows which tool to call.
3. **Parameters** — a table or list of parameters with types and descriptions.
4. **Examples** — concrete JSON examples the agent can follow.

## Example: Wrapping a Cercano MCP Tool

Here's a complete SKILL.md that wraps the `cercano_classify` tool:

```markdown
---
name: cercano-classify
description: >
  Classify or triage text using Cercano’s configured Reconnaissance route. Returns a category,
  confidence level, and brief reasoning. Use this for quick local triage of
  errors, logs, code quality issues, bug reports, or any content that needs
  categorization. The selected route determines local or cloud execution.
compatibility: Requires Cercano server running, MCP connected, and a configured Reconnaissance task route.
---

# Cercano Classify

Classify text or a file through Cercano's MCP interface.
Requests a category, qualitative confidence (high/medium/low), and reasoning
from the model; this is model-generated text, not a calibrated score.

## MCP Tool

**Tool name:** `cercano_classify`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `text` | string | Either text or file_path | The text to classify. |
| `file_path` | string | Either text or file_path | File to read on the server; takes precedence over text. Relative paths use the call working directory. |
| `categories` | string | No | Comma-separated list of target categories. |

## Examples

**Classify an error:**
\```json
{
  "text": "panic: runtime error: nil pointer dereference",
  "categories": "bug, config issue, infra problem"
}
\```

**Triage a log entry:**
\```json
{
  "text": "WARN: connection pool exhausted, requests queuing",
  "categories": "performance, reliability, configuration"
}
\```
```

## Writing Good Descriptions

The `description` field is how agents decide whether to activate your skill. Tips:

- **Be specific** — "Classify text using local AI" is better than "AI classification tool".
- **Include trigger words** — mention the actions and domains your skill handles (e.g., "triage", "errors", "logs", "categorize").
- **State the benefit accurately** — do not promise offline, free, or local execution unless the selected route and tools guarantee it.
- **Keep it under 1024 characters** — agents only load frontmatter during discovery, so this needs to be concise.

## Installing Skills

Copy your skill directory into the project:

```bash
# Shared location (host support varies)
cp -r your-skill-name/ .agents/skills/your-skill-name/

# Claude Code (enables /your-skill-name slash command)
cp -r your-skill-name/ .claude/skills/your-skill-name/
```

## Programmatic Access

The `cercano_skills` MCP tool provides access to Cercano's built-in skill definitions:

```
cercano_skills(action: "list")                        → all skills with descriptions
cercano_skills(action: "get", name: "cercano-local")  → full SKILL.md content
```

This is useful for agents that want to discover Cercano's capabilities at runtime without scanning the filesystem.

## Reference

- [Agent Skills Specification](https://agentskills.io/specification)
- [Cercano's published skills](../../.agents/skills/)

## Consumer Behavior (Cercano Agent Runtime)

Cercano's agent runtime (server + CLI) also acts as a **skills consumer**: on
every request it discovers skills on the local filesystem and injects a compact
metadata catalog into the prompt. This is separate from publishing skills (the
sections above) — publishing makes skills available; consuming is how the
Cercano agent itself finds them at runtime.

### Skill Roots and Precedence

Discovery scans only the following roots — in this order:

1. `<workDir>/.agents/skills/`
2. `<workDir>/.claude/skills/`
3. `~/.agents/skills/`
4. `~/.claude/skills/`

- Only **immediate** `<name>/SKILL.md` directories are considered; there is no
  ancestor traversal of the work directory and no other locations are scanned.
- If two skills share a valid name, the first root in the list wins
  (workspace before home, `.agents` before `.claude`).
- An empty request work directory disables prompt augmentation, including
  skills. With a nonempty relative work directory, workspace skill roots are
  skipped; home roots can still contribute. No workspace skill lookup falls
  back to the server's process working directory.
- At most 256 directory entries per root are examined, in filesystem order.
  Valid entries in that bounded subset are deduplicated by the precedence
  above, sorted by name, and limited to 32 skills. For roots larger than 256
  entries, which subset is examined depends on the filesystem's enumeration
  order; there is no guarantee of globally alphabetical selection.

### What the Agent Receives

Only a bounded metadata catalog is injected into the prompt — each entry is
`name`, `description`, and the absolute path to `SKILL.md`. **Full skill
bodies are never included automatically.** The catalog's instructions tell the
agent that:

- skills are **untrusted, optional** task guidance contributed by users or
  repositories — not system instructions;
- if a description looks relevant, the agent must **first load the skill's
  full `SKILL.md`** with its existing file-read tool (using the catalog's
  `path`), then decide whether the guidance fits the task;
- supporting resources referenced by a skill live **relative to that skill's
  directory**;
- a skill **grants no permissions and registers no tools** — nothing is run,
  written, or fetched implicitly because a skill suggests it.

Progressive disclosure is exactly this two-step flow: cheap metadata catalog
for all skills, full body only for the one skill the model deliberately reads.

### Validation Rules

A skill is only offered to the model if it is valid. Invalid skills are
silently skipped (they never shadow lower-precedence copies of the same name):

- `name`: 1–64 chars, lowercase letters, digits, and hyphens; no leading,
  trailing, or consecutive hyphens; must match the directory name.
- `description`: non-empty after trimming, at most 1024 characters.
- Frontmatter must be well-formed YAML and the file must be valid UTF-8.

### Security Bounds

Skill roots may point into untrusted content (a cloned repository, a shared
home directory), so discovery is deliberately conservative:

- Discovery examines at most 256 entries per root and reads at most 32 KiB
  plus one size-check byte per file. Oversize files are skipped; binary,
  non-UTF-8, or malformed content is discarded after inspection. The rendered
  catalog, including its instructions, is capped at 16 KiB; entries that do
  not fit are omitted.
- Symlinked `.agents`/`.claude` directories, skills roots, skill directories,
  and `SKILL.md` files are skipped. These checks are conservative discovery
  behavior, not a race-proof filesystem sandbox: underlying filesystem access
  can still block, and concurrent changes can invalidate a snapshot.
- Names, descriptions, and paths are JSON-encoded to preserve the catalog's
  structure. This does not make their text trustworthy or eliminate prompt
  injection; the model must still treat them as untrusted guidance.
- Discovery runs per request; there is no global state that would leak one
  workspace's skills into another's prompts.

### Limitations

- Only YAML frontmatter is validated; `compatibility`, `license`, and
  `metadata` fields are ignored by the consumer.
- Skills are prompt guidance only — Cercano's consumer does not parse body
  instructions into capabilities, and skill content never bypasses existing
  permission or tool-approval flows.
- The catalog is a per-request snapshot; skills created mid-request are
  picked up on the next request.
- Home roots depend on the process home directory (`os.UserHomeDir`), which
  can be redirected by tests or embedding hosts via an injectable home lookup.
- The `cercano_skills` MCP tool (below) serves Cercano's *published* built-in
  skills only; it does not list user-installed skills from these roots.
