# Regenerating the gRPC bindings

The schema is `source/proto/agent.proto`; committed Go output lives in
`source/server/pkg/proto/agent.pb.go` and `agent_grpc.pb.go`.
**`source/proto/generate.sh` is the canonical, location-independent procedure.**
It adds `$(go env GOPATH)/bin` to PATH and installs missing Go plugins at pinned
versions. It does not replace an already-present plugin with the pinned version.
Do not run it in a no-install audit unless prerequisites are already present.

## Versions and normal regeneration

The current headers record protoc `v7.34.1` (the executable reports
`libprotoc 34.1`), protoc-gen-go `v1.36.11`, and protoc-gen-go-grpc `v1.6.2`.
Check the headers and script again when regenerating; these versions can change.

```sh
source/proto/generate.sh
```

Before changing the schema, establish a baseline: generation from the existing
schema should reproduce the existing bindings. A zero diff demonstrates output
equivalence, not necessarily identical executable versions. In a dirty worktree,
compare with the preexisting files, not just `git diff` against HEAD.

## Non-mutating, no-install check

From the repository root, with all three executables already installed:

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
protoc --version
protoc-gen-go --version
protoc-gen-go-grpc --version
out=$(mktemp -d)
protoc --proto_path=source/proto \
  --go_out="$out" --go_opt=paths=source_relative \
  --go-grpc_out="$out" --go-grpc_opt=paths=source_relative agent.proto
cmp "$out/agent.pb.go" source/server/pkg/proto/agent.pb.go
cmp "$out/agent_grpc.pb.go" source/server/pkg/proto/agent_grpc.pb.go
rm -r "$out"
```

Stop and explain any baseline difference before mixing regeneration with a schema
change. This check does not run the install-capable script or modify bindings.

## Two recurring traps

1. Adding an RPC changes `proto.AgentClient`. Update implementations such as
   `internal/mcp/server_test.go`'s `mockAgentClient` with the matching stub.
   Production `go build` does not compile test mocks. From `source/server`, run
   `go test ./internal/mcp` and the relevant server/worker tests; `go vet ./...`
   is an additional broader check.
2. Do not trust a textual merge of generated files, especially the serialized
   descriptor. After merging schema changes, regenerate from the merged proto
   and compare. Generated output, not a hand-merged descriptor, is authoritative.

The 2026-10-22 audit performed the temporary-output comparison and test builds;
see [coverage and evidence](../audit-agent-specialists.md).
