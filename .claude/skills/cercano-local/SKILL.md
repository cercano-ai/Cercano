---
name: cercano-local
description: Run prompts through Cercano with configured routing. Handles chat-style queries and code generation with validation. The tool name does not guarantee local, private, free, or faster execution.
compatibility: Requires Cercano server running with a usable configured model route.
---

# Cercano Local Inference

Run prompts through Cercano's MCP interface. Saved routing and redirects determine the destination; this legacy tool name is not a local-only constraint.

## Important: Display the result

MCP tool results may not be visible to the user in the terminal. After calling the tool, you MUST output the full tool result text verbatim in your response so the user can see it.

## Routing and data handling

Saved task routing and redirects determine local or cloud execution. Tool names
do not enforce locality, privacy, cost, or latency. Verify the selected destination
before supplying sensitive input; web research also contacts external search and
page servers. Model output and confidence are not independent verification.

## MCP Tool

**Tool name:** `cercano_local`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | Yes | The prompt to run against local models. |
| `file_path` | string | No | Target file path for code changes. When provided with `work_dir`, enables the agentic code generation loop with validation. |
| `work_dir` | string | No | Working directory for code validation (go build/test). When provided with `file_path`, enables the agentic code generation loop. |
| `context` | string | No | Additional context such as existing code or file contents. |
| `conversation_id` | string | No | Conversation ID for multi-turn support across calls. |

## Modes

### Chat Mode
Provide only `prompt` (and optionally `context`) for a direct LLM call. The response is the model's text output.

### Agentic Code Generation Mode
Provide `prompt`, `file_path`, and `work_dir` to enable a generate-validate loop. Cercano will:
1. Generate code based on the prompt
2. Write it to the target file
3. Run validation (build/test) in the working directory
4. If validation fails, self-correct and retry

## Examples

**Chat query:**
```json
{
  "prompt": "What are the SOLID principles in software design?"
}
```

**Code generation with context:**
```json
{
  "prompt": "Add error handling to this function",
  "file_path": "internal/handler/auth.go",
  "work_dir": "/project",
  "context": "func Login(w http.ResponseWriter, r *http.Request) { ... }"
}
```

**Multi-turn conversation:**
```json
{
  "prompt": "Now refactor that to use the repository pattern",
  "conversation_id": "conv-abc123"
}
```
