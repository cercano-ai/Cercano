# IDE integration over ACP

Cercano provides an Agent Client Protocol (ACP) **v1** stdio adapter. Each editor starts its own adapter process; adapters connect to the same local gRPC agent used by the terminal client. Tool execution, conversation storage, model routing, and permission policy remain in that shared server.

## Run the adapter

Configure an ACP-capable editor or extension to launch the **server executable** with the `acp` argument:

```json
{
  "command": "/absolute/path/to/cercano",
  "args": ["acp"]
}
```

The exact configuration wrapper depends on the editor. This is a command/arguments example, not a universal editor settings file. Stock VS Code is not assumed to provide generic ACP registration; use an ACP client extension.

For a source build:

```sh
# From the repository root:
(cd source/server && go build -o bin/cercano ./cmd/cercano/)
```

Then use the absolute path to `source/server/bin/cercano`. A development launcher named `cercano` may dispatch to the terminal UI instead of the server executable; configure the real binary, not that wrapper. In the development install this is normally `~/bin/.cercano-libexec/cercano`.

Standard output is reserved for protocol messages. Diagnostics go to standard error. The explicit `acp` argument is required: bare `cercano` starts the normal server regardless of stdin, without automatic ACP detection.

## Shared server and permissions

The adapter connects to the configured local server, auto-launching one if needed through Cercano's existing client connection mechanism. Closing an editor closes its adapter, not a manually managed server or another editor's adapter.

**The server's permission policy applies unchanged.** The adapter forwards approval requests emitted by the server and sends the editor's decision back to that conversation. It does not switch the daemon's permission mode, establish a stricter per-editor mode, or guarantee that every mutation causes a prompt. In particular, a permissive or bypass server policy may allow operations without asking the editor.

Only explicit, recognized approval choices authorize a pending request. Cancellation, malformed replies, and unknown outcomes must not become approval.

## Initial scope

The initial adapter supports initialization, new sessions, text and resource-link prompts, streamed responses and tool activity, approval round-trips, and cancellation. Resource links are passed to the agent as named URI references; the adapter does not read or fetch their contents itself. It does not advertise ACP v2, session history loading, or editor-provided MCP tool support. Unsupported prompt content and unsupported nonempty MCP server configurations are rejected rather than silently ignored. Authentication and other workflows requiring unsupported interaction need to be resolved through Cercano's existing clients.

Capability advertisements are the authoritative description of what the running binary supports. Protocol-level tests do not establish that every editor UI renders every event identically.

A connection admits at most 64 outstanding prompts, with at most one prompt per session until its final response is delivered. Excess requests receive an error without cancelling existing sessions. Terminal-response capacity is reserved at admission so concurrent completions cannot exhaust an unreserved output queue.

Closing stdin ends the connection and cancels active work; it is not a way to wait for a long-running prompt. Already-dispatched synchronous replies get a bounded 250-millisecond drain window. An undrainable reply produces a diagnostic error rather than hanging shutdown. Cancellation of a session releases its execution and pending permission decisions even if the editor has stopped reading output.

## Noninteractive verification

From `source/server`, run:

```sh
go test -race ./internal/acp/... ./cmd/cercano -count=1
go test ./pkg/agentclient ./internal/server ./internal/mcp -count=1
go vet ./internal/acp/... ./cmd/cercano
go build -o /tmp/cercano-acp-check ./cmd/cercano/
```

The ACP tests use injected backends and a local test gRPC service; they require no model credentials or editor interaction. They cover streamed output, tool activity, permission allow/deny decisions, cancellation, disconnect, concurrent sessions, resource-link forwarding, and output backpressure. These are protocol and adapter checks, not live-model or editor UI acceptance tests.

## Version boundaries

The implementation keeps stdio transport, v1 wire messages, and backend execution separate. This preserves room for a future v2 adapter without changing the server's execution ownership or pretending that the two wire protocols are interchangeable.

ACP v2 is still a draft. In particular, its prompt response acknowledges actual user-message insertion, while completion is delivered through session state notifications. A future implementation needs a reliable backend insertion acknowledgment, session-wide event handling, and the complete v2 baseline session lifecycle before advertising v2 session support. Adding a version number or mechanically renaming fields is not sufficient.

Reference: [ACP v2 migration guide](https://agentclientprotocol.com/protocol/v2/migration).
