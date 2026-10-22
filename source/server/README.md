# Cercano server

This Go module builds `cercano`, the agent gRPC server and MCP host. The terminal
client is a separate module, `source/clients/cli`, and binary, `cercano-cli`.
Bare `cercano` is not the installed terminal UI.

## Build from this checkout

Both Go modules require Go 1.26 (see their `go.mod` files). From the repository root:

```bash
make -C source/server build
make -C source/clients/cli build
```

The outputs are `source/server/bin/cercano` and
`source/clients/cli/bin/cercano-cli`. Co-locate them for the CLI's sibling-server
auto-launch, or use the [development launcher](../../docs/agent/self-dev.md).
`make build` supplies version metadata and invokes optional macOS signing.

For packaged installation and supported platforms, start at the
[root README](../../README.md#get-started). For configuration and commands, see
[the agent guide](../../docs/agent/README.md).

## Inference

The default `open_runtime` is `llama_server`, a managed llama.cpp child process.
Optional `mistralrs` and Ollama backends are also supported. Ollama is not a
mandatory prerequisite. Runtime binaries and compatible model files are separate
from building the agent. Configure Runtime / Local Models and task routing in the
CLI; see the [runtime guide](../../docs/agent/features/integrated-local-runtime.md).
Cloud profiles are configured independently in Cloud and Routing.

## Development

- `cmd/cercano/`: unified server/command entry point.
- `internal/server/`: gRPC service implementation.
- `internal/agent/`: tool loop and agent orchestration.
- `internal/llm/`, `internal/inference/`: providers and inference policy.
- `internal/localruntime/`: managed local processes and model lifecycle.
- `pkg/`: exported config, protocol, client and update packages shared with the CLI.

Run unit tests from this module with `go test ./...`. Regenerating protocol
bindings is separate from ordinary builds; see [proto regeneration](../../docs/agent/proto-regen.md)
for the pinned tooling rather than installing arbitrary latest plugin versions.
