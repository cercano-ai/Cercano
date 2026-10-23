---
name: cercano-config
description: Query or update Cercano's runtime configuration without restarting the server. Use action 'get' to list available local models. Use action 'set' to switch the active local runtime or model, change the Ollama endpoint URL, or change legacy cloud-provider settings. Cloud model selection now uses profile quality choices.
compatibility: Requires Cercano server running.
---

# Cercano Config

Query or update Cercano's runtime configuration.

## MCP Tool

**Tool name:** `cercano_config`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | Yes | `"get"` to list available local models, `"set"` to update config. |
| `local_runtime` | string | No | Local runtime to use for generation: `"ollama"`, `"llama_server"`, or `"mistralrs"`. |
| `local_model` | string | No | Local model name to set (e.g. `"qwen2.5-coder:32b"`). |
| `cloud_provider` | string | No | Cloud provider to set: `"google"` or `"anthropic"`. |
| `cloud_model` | string | No | Retired: the server rejects this field; edit profile quality choices instead. |
| `ollama_url` | string | No | Ollama endpoint URL (e.g. `"http://mac-studio.local:11434"`). |

## Examples

**Switch local model:**
```json
{
  "action": "set",
  "local_model": "qwen2.5-coder:32b"
}
```

**Switch local runtime:**
```json
{
  "action": "set",
  "local_runtime": "llama_server"
}
```

**Point to a different Ollama instance:**
```json
{
  "action": "set",
  "ollama_url": "http://mac-studio.local:11434"
}
```

Cloud model selection is not supported by this legacy tool field. Use the current
profile/account configuration UI; `cloud_model` returns a retired-field error.
